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

	nav, name, ok := loadDocsJSON(docsDir, "")
	if !ok {
		t.Fatal("loadDocsJSON did not find docs.json in a parent directory")
	}
	if name != "example" {
		t.Errorf("name = %q, want %q", name, "example")
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
