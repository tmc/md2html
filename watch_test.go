package md2html

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestWatchEnabled(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		source  string
		want    bool
		wantErr bool
	}{
		{"auto directory listing", "auto", "", true, false},
		{"auto source file", "auto", "README.md", true, false},
		{"auto stdin", "auto", "-", false, false},
		{"empty mode", "", "", true, false},
		{"false", "false", "README.md", false, false},
		{"true stdin", "true", "-", true, false},
		{"invalid", "maybe", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := watchEnabled(tt.mode, tt.source)
			if (err != nil) != tt.wantErr {
				t.Fatalf("watchEnabled() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("watchEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReloadNavigationPicksUpDocsJSON checks that editing docs.json is
// reflected without restarting the server: a reordered sidebar or a
// renamed site appears on the next request.
func TestReloadNavigationPicksUpDocsJSON(t *testing.T) {
	siteDir := t.TempDir()
	docsDir := filepath.Join(siteDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		body := "---\ntitle: Page " + strings.ToUpper(name) + "\n---\n\n# " + name + "\n"
		if err := os.WriteFile(filepath.Join(docsDir, name+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(siteDir, "docs.json")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"name":"First","navigation":{"groups":[{"group":"G","pages":["docs/a"]}]}}`)

	s := &server{
		config:    Config{Nav: true, Source: docsDir},
		logger:    discardLogger(),
		navRoot:   docsDir,
		navConfig: config,
		navTitle:  defaultTitle,
	}
	nav, site, err := navigationForDir(docsDir, "")
	if err != nil {
		t.Fatal(err)
	}
	s.nav, s.site = nav, site
	s.title = siteTitle(s.navTitle, site.Name)
	if got := len(s.nav.Flat); got != 1 {
		t.Fatalf("initial navigation has %d pages, want 1", got)
	}

	// A second page and a new site name.
	write(`{"name":"Second","navigation":{"groups":[{"group":"G","pages":["docs/a","docs/b"]}]}}`)
	s.reloadNavigation(context.Background())
	if got := len(s.nav.Flat); got != 2 {
		t.Errorf("navigation has %d pages after reload, want 2", got)
	}
	if s.site.Name != "Second" {
		t.Errorf("site name = %q after reload, want %q", s.site.Name, "Second")
	}
	// The title follows the renamed site. The name applied at startup
	// is not a title the caller chose, so it does not outrank the new one.
	if got := s.siteTitle(); got != "Second" {
		t.Errorf("title = %q after reload, want %q", got, "Second")
	}

	// A config saved mid-edit must not empty the sidebar.
	write(`{"name":"Second","navigation":{`)
	s.reloadNavigation(context.Background())
	if got := len(s.nav.Flat); got != 2 {
		t.Errorf("navigation has %d pages after an unparsable config, want the previous 2", got)
	}
}

// TestReloadNavigationKeepsStars checks that the star count survives a
// reload of the file that names the repository. Refetching on every save
// would spend the API rate limit on keystrokes.
func TestReloadNavigationKeepsStars(t *testing.T) {
	siteDir := t.TempDir()
	docsDir := filepath.Join(siteDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(siteDir, "docs.json")
	body := `{"name":"S","navbar":{"primary":{"type":"github","href":"https://github.com/tmc/cdp"}},"navigation":{"groups":[{"group":"G","pages":["docs/a"]}]}}`
	if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &server{
		config:    Config{Nav: true, Source: docsDir, Stars: true},
		logger:    discardLogger(),
		navRoot:   docsDir,
		navConfig: config,
	}
	nav, site, err := navigationForDir(docsDir, "")
	if err != nil {
		t.Fatal(err)
	}
	site.Stars = "4" // as if fetched at startup
	s.nav, s.site = nav, site

	// githubAPI is left pointing at the real host: a reload that kept
	// the count makes no request at all, which is the point.
	s.reloadNavigation(context.Background())
	if s.site.Stars != "4" {
		t.Errorf("stars = %q after reloading the same repository, want %q", s.site.Stars, "4")
	}
}

// TestReloadNavigationDuringRequests exercises a reload against the reads
// a request performs. The navigation, the site, and the title are rebuilt
// on the watch goroutine while pages are being served, so every one of
// them has to be written under the lock that the request paths read it
// with. Under -race an unguarded field fails here; without a test that
// overlaps the two, it does not fail anywhere.
func TestReloadNavigationDuringRequests(t *testing.T) {
	siteDir := t.TempDir()
	docsDir := filepath.Join(siteDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(siteDir, "docs.json")
	write := func(name string) {
		body := `{"name":"` + name + `","navigation":{"groups":[{"group":"G","pages":["docs/a"]}]}}`
		if err := os.WriteFile(config, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("First")

	s := &server{
		config:    Config{Nav: true, Source: docsDir},
		logger:    discardLogger(),
		navRoot:   docsDir,
		navConfig: config,
		navTitle:  defaultTitle,
	}
	nav, site, err := navigationForDir(docsDir, "")
	if err != nil {
		t.Fatal(err)
	}
	s.nav, s.site = nav, site
	s.title = siteTitle(s.navTitle, site.Name)

	// The reader runs until the reloads finish rather than for a fixed
	// count: a reload does file I/O and is much slower, so a fixed count
	// races through and leaves the two goroutines barely overlapping.
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer close(done)
		for i := 0; i < 50; i++ {
			write("Site" + string(rune('A'+i%26)))
			s.reloadNavigation(context.Background())
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			// What a request reads: the title for the page head, and the
			// navigation and site for the sidebar and the header.
			_ = s.siteTitle()
			s.mu.RLock()
			_, _ = s.nav, s.site
			s.mu.RUnlock()
		}
	}()
	wg.Wait()

	if s.siteTitle() == "" {
		t.Error("title was emptied by concurrent reloads")
	}
}

// TestWatchEventReloadsNavigationItems checks that the sidebar follows
// the pages it lists: an edited title, a new page, and a page in a
// directory created after startup all appear without a restart.
func TestWatchEventReloadsNavigationItems(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) string {
		t.Helper()
		name := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return name
	}
	a := write("a.md", "---\ntitle: Before\n---\n\n# A\n")
	write(".hidden/x.md", "# X\n")

	s := &server{
		config:  Config{Nav: true},
		logger:  discardLogger(),
		clients: make(map[chan string]bool),
		navRoot: root,
	}
	nav, site, err := navigationForDir(root, "")
	if err != nil {
		t.Fatal(err)
	}
	s.nav, s.site = nav, site
	watcher, err := s.startWatching()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if s.watched[filepath.Join(root, ".hidden")] {
		t.Error("watching a directory the navigation skips")
	}

	titles := func() string {
		var out []string
		for _, item := range s.nav.Flat {
			out = append(out, item.Title)
		}
		return strings.Join(out, ",")
	}
	ctx := context.Background()

	write("a.md", "---\ntitle: After\n---\n\n# A\n")
	s.handleWatchEvent(ctx, fsnotify.Event{Name: a, Op: fsnotify.Write})
	if got := titles(); got != "After" {
		t.Errorf("titles = %q after editing a page, want %q", got, "After")
	}

	b := write("b.md", "---\ntitle: New\n---\n\n# B\n")
	s.handleWatchEvent(ctx, fsnotify.Event{Name: b, Op: fsnotify.Create})
	if got := titles(); got != "After,New" {
		t.Errorf("titles = %q after adding a page, want %q", got, "After,New")
	}

	write("guide/c.md", "---\ntitle: Nested\n---\n\n# C\n")
	guide := filepath.Join(root, "guide")
	s.handleWatchEvent(ctx, fsnotify.Event{Name: guide, Op: fsnotify.Create})
	if !s.watched[guide] {
		t.Error("new directory is not watched")
	}
	if got := titles(); !strings.Contains(got, "Nested") {
		t.Errorf("titles = %q after adding a directory, want Nested among them", got)
	}
}

func TestWatchEventReloadsIncludedFile(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	examples := filepath.Join(root, "examples")
	for _, dir := range []string{docs, examples} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	program := filepath.Join(examples, "main.go")
	if err := os.WriteFile(program, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	site, err := prepareSite(Config{Source: docs, IncludeRoot: root}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := site.prepareIncludes(docs, docs); err != nil {
		t.Fatal(err)
	}
	client := make(chan string, 1)
	s := &server{
		config:    site.config,
		prepared:  site,
		logger:    discardLogger(),
		clients:   map[chan string]bool{client: true},
		inputPath: docs,
	}
	watcher, err := s.startWatching()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if s.watched[examples] {
		t.Fatal("watching a directory before a page includes a file from it")
	}

	html, err := site.markdownToHTML("<Code src=\"/examples/main.go\" />\n", "page.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "package") {
		t.Fatalf("page does not show the included file:\n%s", html)
	}
	if !s.watched[examples] {
		t.Error("directory of an included file is not watched")
	}

	ctx := context.Background()
	s.handleWatchEvent(ctx, fsnotify.Event{Name: filepath.Join(examples, "other.go"), Op: fsnotify.Write})
	select {
	case <-client:
		t.Error("reloaded for a file no page includes")
	default:
	}
	s.handleWatchEvent(ctx, fsnotify.Event{Name: program, Op: fsnotify.Write})
	select {
	case <-client:
	default:
		t.Error("no reload after an included file changed")
	}
}
