package md2html

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBuildSearchIndex(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"index.md":         "# Home\n",
		"guide/start.mdx":  "# Start\n",
		"guide/draft.md":   "---\ndraft: true\n---\n# Draft\n",
		"SUMMARY.md":       "# Summary\n\n- [Home](index.md)\n",
		"private/notes.md": "# Notes\n",
		".md2htmlignore":   "private/\n",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		cfg  Config
		want []string
	}{
		{"server", Config{}, []string{"guide/start", "index"}},
		{"static", Config{HTMLExt: "html"}, []string{"guide/start.html", "index.html"}},
		{"dirs", Config{HTMLExt: "html", HTMLLayout: "dir"}, []string{"./", "guide/start/"}},
		{"drafts", Config{Drafts: true}, []string{"guide/draft", "guide/start", "index"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := buildSearchIndex(root, tt.cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, doc := range docs {
				got = append(got, doc.URL)
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("urls = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSearchIndexTitles(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"named.md":   "---\ntitle: From Frontmatter\n---\n# A Heading\n",
		"heading.md": "# Getting Started\n",
		"bare.mdx":   "Just text.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	docs, err := buildSearchIndex(root, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	for _, doc := range docs {
		got[doc.URL] = doc.Title
	}
	want := map[string]string{"named": "From Frontmatter", "heading": "Getting Started", "bare": "bare"}
	if !maps.Equal(got, want) {
		t.Errorf("titles = %q, want %q", got, want)
	}
}
