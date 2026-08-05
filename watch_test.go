package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
// reflected without restarting the server. Navigation used to be built
// once at startup, so a reordered sidebar or a renamed site needed a
// restart to appear.
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
	s.config.Title = siteTitle(s.navTitle, site.Name)
	if got := len(s.nav.Flat); got != 1 {
		t.Fatalf("initial navigation has %d pages, want 1", got)
	}

	// A second page and a new site name.
	write(`{"name":"Second","navigation":{"groups":[{"group":"G","pages":["docs/a","docs/b"]}]}}`)
	s.reloadNavigation()
	if got := len(s.nav.Flat); got != 2 {
		t.Errorf("navigation has %d pages after reload, want 2", got)
	}
	if s.site.Name != "Second" {
		t.Errorf("site name = %q after reload, want %q", s.site.Name, "Second")
	}
	// The title has to follow the renamed site. It used to stick,
	// because the name applied at startup was mistaken for a title the
	// caller had chosen.
	if s.config.Title != "Second" {
		t.Errorf("title = %q after reload, want %q", s.config.Title, "Second")
	}

	// A config saved mid-edit must not empty the sidebar.
	write(`{"name":"Second","navigation":{`)
	s.reloadNavigation()
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
	s.reloadNavigation()
	if s.site.Stars != "4" {
		t.Errorf("stars = %q after reloading the same repository, want %q", s.site.Stars, "4")
	}
}
