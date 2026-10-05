package markdownfile

import "testing"

func TestIs(t *testing.T) {
	tests := []struct {
		name    string
		is      bool
		trimmed string
		index   bool
	}{
		{"guide.md", true, "guide", false},
		{"guide.markdown", true, "guide", false},
		{"guide.mdx", true, "guide", false},
		{"docs/Guide.MDX", true, "docs/Guide", false},
		{"index.mdx", true, "index", true},
		{"docs/index.md", true, "docs/index", true},
		{"reindex.md", true, "reindex", false},
		{"main.go", false, "main.go", false},
		{"index.html", false, "index.html", false},
		{"md", false, "md", false},
		{"", false, "", false},
	}
	for _, tt := range tests {
		if got := Is(tt.name); got != tt.is {
			t.Errorf("Is(%q) = %v, want %v", tt.name, got, tt.is)
		}
		if got := TrimExt(tt.name); got != tt.trimmed {
			t.Errorf("TrimExt(%q) = %q, want %q", tt.name, got, tt.trimmed)
		}
		if got := IsIndex(tt.name); got != tt.index {
			t.Errorf("IsIndex(%q) = %v, want %v", tt.name, got, tt.index)
		}
	}
}
