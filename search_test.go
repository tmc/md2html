package md2html

import (
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
		{"drafts", Config{Drafts: true}, []string{"guide/draft", "guide/start", "index"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := buildSearchIndex(root, tt.cfg)
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
