package md2html

import (
	"os"
	"path/filepath"
	"testing"
)

// writeDocsSite lays out a site root holding docs.json with the Markdown
// under docs/, the layout Mintlify projects use and the one that makes
// page paths differ from paths relative to the served directory.
func writeDocsSite(t *testing.T, config string) (siteDir, docsDir string) {
	t.Helper()
	siteDir = t.TempDir()
	docsDir = filepath.Join(siteDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "docs.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{
		"index.md":      "---\ntitle: Home\n---\n\n# Ignored\n",
		"quickstart.md": "# Quickstart\n",
		"deep.md":       "# Deep\n",
	}
	for name, body := range pages {
		if err := os.WriteFile(filepath.Join(docsDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return siteDir, docsDir
}

const testDocsJSON = `{
  "name": "example",
  "colors": {"primary": "#4F46E5", "light": "#A5B4FC"},
  "navigation": {
    "groups": [
      {"group": "Start here", "pages": ["docs/index", "docs/quickstart"]},
      {"group": "Nested", "pages": [{"group": "Inner", "pages": ["docs/deep"]}]},
      {"group": "Elsewhere", "pages": ["other/page"]},
      {"group": "Missing", "pages": ["docs/nope"]}
    ]
  }
}`

func TestLoadDocsJSON(t *testing.T) {
	_, docsDir := writeDocsSite(t, testDocsJSON)

	nav, site, ok := loadDocsJSON(docsDir, "")
	if !ok {
		t.Fatal("loadDocsJSON did not find docs.json in a parent directory")
	}
	if site.Name != "example" {
		t.Errorf("name = %q, want %q", site.Name, "example")
	}
	if site.Accent != "#4F46E5" {
		t.Errorf("accent = %q, want %q", site.Accent, "#4F46E5")
	}
	// "light" is the variant for dark backgrounds.
	if site.AccentDark != "#A5B4FC" {
		t.Errorf("dark accent = %q, want %q", site.AccentDark, "#A5B4FC")
	}

	// Groups naming no reachable page are dropped, so "Elsewhere" (outside
	// the served directory) and "Missing" (no such file) do not appear.
	if got, want := len(nav.Items), 2; got != want {
		var titles []string
		for _, item := range nav.Items {
			titles = append(titles, item.Title)
		}
		t.Fatalf("top-level items = %d %v, want %d", got, titles, want)
	}

	start := nav.Items[0]
	if !start.IsGroup || start.Title != "Start here" {
		t.Fatalf("first item = %+v, want the group \"Start here\"", start)
	}
	if got, want := len(start.Children), 2; got != want {
		t.Fatalf("group children = %d, want %d", got, want)
	}
	// Frontmatter title wins; otherwise the first heading.
	if got, want := start.Children[0].Title, "Home"; got != want {
		t.Errorf("first page title = %q, want %q", got, want)
	}
	if got, want := start.Children[1].Title, "Quickstart"; got != want {
		t.Errorf("second page title = %q, want %q", got, want)
	}
	// Paths are relative to the served directory, not the site root.
	if got, want := start.Children[0].Path, "index.md"; got != want {
		t.Errorf("first page path = %q, want %q", got, want)
	}

	nested := nav.Items[1]
	if len(nested.Children) != 1 || !nested.Children[0].IsGroup {
		t.Fatalf("nested group not preserved: %+v", nested)
	}
	if got, want := nested.Children[0].Children[0].Path, "deep.md"; got != want {
		t.Errorf("nested page path = %q, want %q", got, want)
	}

	// Prev/next ordering follows docs.json, not the file system.
	if got, want := len(nav.Flat), 3; got != want {
		t.Fatalf("flat pages = %d, want %d", got, want)
	}
}

// TestLoadDocsJSONFromSiteRoot checks the other layout: the server is
// started at the site root, so page paths need no adjustment.
func TestLoadDocsJSONFromSiteRoot(t *testing.T) {
	siteDir, _ := writeDocsSite(t, testDocsJSON)

	nav, _, ok := loadDocsJSON(siteDir, "")
	if !ok {
		t.Fatal("loadDocsJSON did not find docs.json in the directory itself")
	}
	if got, want := nav.Items[0].Children[0].Path, "docs/index.md"; got != want {
		t.Errorf("page path = %q, want %q", got, want)
	}
}

func TestLoadDocsJSONNavbarLinks(t *testing.T) {
	_, docsDir := writeDocsSite(t, `{
	  "navigation": {"groups": [{"group": "G", "pages": ["docs/index"]}]},
	  "navbar": {
	    "links": [
	      {"label": "Community", "href": "https://example.com/chat"},
	      {"label": "", "href": "https://example.com/skipped"},
	      {"label": "No href"}
	    ],
	    "primary": {"type": "github", "href": "https://github.com/o/r"}
	  }
	}`)

	_, site, ok := loadDocsJSON(docsDir, "")
	if !ok {
		t.Fatal("loadDocsJSON failed")
	}
	want := []SiteLink{{Label: "Community", Href: "https://example.com/chat"}}
	if len(site.Links) != 1 || site.Links[0] != want[0] {
		t.Errorf("links = %+v, want %+v", site.Links, want)
	}
	if site.Repo != "o/r" {
		t.Errorf("repo = %q, want o/r", site.Repo)
	}
}

// TestLoadDocsJSONRelativeDir checks that a relative source directory,
// as a plain "md2html -html out ." invocation produces, still resolves
// pages against the absolute docs.json directory.
func TestLoadDocsJSONRelativeDir(t *testing.T) {
	siteDir, _ := writeDocsSite(t, testDocsJSON)
	t.Chdir(siteDir)

	nav, _, ok := loadDocsJSON(".", "")
	if !ok {
		t.Fatal("loadDocsJSON did not load docs.json for a relative source directory")
	}
	if got, want := nav.Items[0].Children[0].Path, "docs/index.md"; got != want {
		t.Errorf("page path = %q, want %q", got, want)
	}
}

func TestLoadDocsJSONAbsent(t *testing.T) {
	// t.TempDir is under the system temp directory, which has no docs.json
	// above it in any tree this test controls.
	dir := t.TempDir()
	if _, _, ok := loadDocsJSON(dir, ""); ok {
		t.Error("loadDocsJSON reported a docs.json for a directory that has none")
	}
}

func TestLoadDocsJSONMalformed(t *testing.T) {
	_, docsDir := writeDocsSite(t, "{not json")
	if _, _, ok := loadDocsJSON(docsDir, ""); ok {
		t.Error("loadDocsJSON accepted a malformed docs.json")
	}
}

// TestLoadDocsJSONRejectsBadColor checks that a color md2html will not
// interpolate into a stylesheet is dropped rather than escaped, leaving
// the built-in accent in place.
func TestLoadDocsJSONRejectsBadColor(t *testing.T) {
	config := `{"name": "x", "colors": {"primary": "red; } body { display:none"},
	  "navigation": {"groups": [{"group": "G", "pages": ["docs/index"]}]}}`
	_, docsDir := writeDocsSite(t, config)
	_, site, ok := loadDocsJSON(docsDir, "")
	if !ok {
		t.Fatal("loadDocsJSON failed")
	}
	if site.Accent != "" {
		t.Errorf("accent = %q, want it dropped", site.Accent)
	}
}

func TestSiteTitle(t *testing.T) {
	tests := []struct {
		configured string
		siteName   string
		want       string
	}{
		{defaultTitle, "example", "example"},
		{"", "example", "example"},
		{"Mine", "example", "Mine"},
		{defaultTitle, "", defaultTitle},
	}
	for _, tt := range tests {
		if got := siteTitle(tt.configured, tt.siteName); got != tt.want {
			t.Errorf("siteTitle(%q, %q) = %q, want %q", tt.configured, tt.siteName, got, tt.want)
		}
	}
}

// TestLoadDocsJSONIcons checks that icons reach the navigation from each
// page's frontmatter. Mintlify's docs.json never names an icon: its
// generated nav is decorated from the pages themselves.
func TestLoadDocsJSONIcons(t *testing.T) {
	siteDir, docsDir := writeDocsSite(t, `{
  "name": "example",
  "navigation": {
    "groups": [{"group": "Start here", "pages": ["docs/index", "docs/quickstart"]}]
  }
}`)
	if err := os.WriteFile(filepath.Join(docsDir, "index.md"),
		[]byte("---\ntitle: Home\nicon: book-open\n---\n\n# Home\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = siteDir

	nav, _, ok := loadDocsJSON(docsDir, "")
	if !ok {
		t.Fatal("loadDocsJSON did not find docs.json")
	}
	want := map[string]string{"Home": "book-open", "Quickstart": ""}
	for _, item := range nav.Flat {
		if w, tracked := want[item.Title]; tracked && item.Icon != w {
			t.Errorf("%s icon = %q, want %q", item.Title, item.Icon, w)
		}
	}
}

func TestRepoLink(t *testing.T) {
	tests := []struct {
		name     string
		link     docsJSONNavbarLink
		wantRepo string
		wantURL  string
	}{
		{
			name:     "github repository",
			link:     docsJSONNavbarLink{Type: "github", Href: "https://github.com/tmc/cdp"},
			wantRepo: "tmc/cdp",
			wantURL:  "https://github.com/tmc/cdp",
		},
		{
			name:     "trailing path is dropped",
			link:     docsJSONNavbarLink{Type: "github", Href: "https://github.com/tmc/cdp/tree/main/docs"},
			wantRepo: "tmc/cdp",
			wantURL:  "https://github.com/tmc/cdp",
		},
		{
			name:     "git suffix is dropped",
			link:     docsJSONNavbarLink{Type: "github", Href: "https://github.com/tmc/cdp.git"},
			wantRepo: "tmc/cdp",
			wantURL:  "https://github.com/tmc/cdp",
		},
		// A link md2html cannot read as a repository shows nothing, since
		// the label is meant to read as "owner/name".
		{name: "another link type", link: docsJSONNavbarLink{Type: "button", Href: "https://github.com/tmc/cdp"}},
		{name: "not github", link: docsJSONNavbarLink{Type: "github", Href: "https://example.com/tmc/cdp"}},
		{name: "owner only", link: docsJSONNavbarLink{Type: "github", Href: "https://github.com/tmc"}},
		{name: "empty", link: docsJSONNavbarLink{}},
		{name: "not https", link: docsJSONNavbarLink{Type: "github", Href: "http://github.com/tmc/cdp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, url := repoLink(tt.link)
			if repo != tt.wantRepo || url != tt.wantURL {
				t.Errorf("repoLink() = %q, %q, want %q, %q", repo, url, tt.wantRepo, tt.wantURL)
			}
		})
	}
}

// TestLoadDocsJSONRepo checks that the repository reaches the site info
// the templates render from.
func TestLoadDocsJSONRepo(t *testing.T) {
	_, docsDir := writeDocsSite(t, `{
  "name": "example",
  "navbar": {"primary": {"type": "github", "href": "https://github.com/tmc/cdp"}},
  "navigation": {"groups": [{"group": "G", "pages": ["docs/index"]}]}
}`)
	_, site, ok := loadDocsJSON(docsDir, "")
	if !ok {
		t.Fatal("loadDocsJSON did not find docs.json")
	}
	if site.Repo != "tmc/cdp" || site.RepoURL != "https://github.com/tmc/cdp" {
		t.Errorf("repo = %q, %q, want %q, %q", site.Repo, site.RepoURL, "tmc/cdp", "https://github.com/tmc/cdp")
	}
}

// TestIconLibraryForSource characterizes the docs.json lookup rendering
// does. mdvet does its own, in documentIcons, and the two differ where
// each has reason to: rendering matches the library name exactly and
// stops at the first docs.json it finds, while vetting folds case and
// keeps looking past a file it cannot read, so that a diagnostic always
// names a config it managed to open. The duplication is small and the
// behavior is not shared, so neither is factored into the other.
func TestIconLibraryForSource(t *testing.T) {
	tests := []struct {
		name string
		// config is the docs.json written at the site root, or ""
		// to write none.
		config string
		want   string
	}{
		{"no docs.json", "", "fontawesome"},
		{"no library named", `{"name": "example"}`, "fontawesome"},
		{"lucide", `{"icons": {"library": "lucide"}}`, "lucide"},
		{"tabler", `{"icons": {"library": "tabler"}}`, "tabler"},
		{"fontawesome", `{"icons": {"library": "fontawesome"}}`, "fontawesome"},
		{"unknown library", `{"icons": {"library": "heroicons"}}`, "fontawesome"},
		{"library name is case sensitive", `{"icons": {"library": "Lucide"}}`, "fontawesome"},
		{"malformed", `{"icons": `, "fontawesome"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			docs := filepath.Join(root, "docs")
			if err := os.MkdirAll(docs, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.config != "" {
				if err := os.WriteFile(filepath.Join(root, docsJSONName), []byte(tt.config), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			page := filepath.Join(docs, "index.md")
			if err := os.WriteFile(page, []byte("# Hi\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			// A docs.json above the source still governs it, and a
			// source naming a file is read as the directory holding it.
			for _, source := range []string{docs, page} {
				if got := iconLibraryForSource("", source); got != tt.want {
					t.Errorf("iconLibraryForSource(%q) = %q, want %q", source, got, tt.want)
				}
			}
		})
	}
}
