package md2html

import "testing"

func TestPageTitle(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter map[string]any
		filePath    string
		fallback    string
		want        string
	}{
		{"frontmatter", map[string]any{"title": "Page"}, "guide.md", "Site", "Page"},
		{"filename", nil, "docs/guide.md", "Site", "guide"},
		{"fallback", nil, ".md", "Site", "Site"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pageTitle(tt.frontmatter, tt.filePath, tt.fallback); got != tt.want {
				t.Fatalf("pageTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
