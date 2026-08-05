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
