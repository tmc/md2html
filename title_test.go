package md2html

import "testing"

func TestPromoteTitleHeading(t *testing.T) {
	tests := []struct {
		name string
		doc  DocumentData
		want string
	}{
		{
			name: "title becomes the heading",
			doc: DocumentData{
				Frontmatter: map[string]any{"title": "Getting started"},
				Content:     "Install the tool.\n",
			},
			want: "# Getting started\n\nInstall the tool.\n",
		},
		{
			name: "existing heading is kept",
			doc: DocumentData{
				Frontmatter: map[string]any{"title": "Getting started"},
				Content:     "# Already here\n\ntext\n",
			},
			want: "# Already here\n\ntext\n",
		},
		{
			name: "no title leaves the body alone",
			doc:  DocumentData{Frontmatter: map[string]any{}, Content: "text\n"},
			want: "text\n",
		},
		{
			name: "leading blank lines are skipped",
			doc: DocumentData{
				Frontmatter: map[string]any{"title": "T"},
				Content:     "\n\n# H\n",
			},
			want: "\n\n# H\n",
		},
		{
			name: "deeper heading still gets the title",
			doc: DocumentData{
				Frontmatter: map[string]any{"title": "T"},
				Content:     "## Section\n",
			},
			want: "# T\n\n## Section\n",
		},
		{
			name: "description becomes the lede",
			doc: DocumentData{
				Frontmatter: map[string]any{"title": "T", "description": "What this is."},
				Content:     "Body.\n",
			},
			want: "# T\n\nWhat this is.\n\nBody.\n",
		},
		{
			name: "description follows an existing heading",
			doc: DocumentData{
				Frontmatter: map[string]any{"description": "What this is."},
				Content:     "# Already here\n\nBody.\n",
			},
			want: "# Already here\n\nWhat this is.\n\nBody.\n",
		},
		{
			name: "description already in the body is not repeated",
			doc: DocumentData{
				Frontmatter: map[string]any{"title": "T", "description": "Body."},
				Content:     "Body.\n",
			},
			want: "# T\n\nBody.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := promoteTitleHeading(tt.doc); got != tt.want {
				t.Errorf("promoteTitleHeading() = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestDocumentTitle(t *testing.T) {
	tests := []struct {
		name     string
		doc      DocumentData
		filePath string
		want     string
	}{
		{
			"frontmatter wins over heading",
			DocumentData{Content: "# Heading\n", Frontmatter: map[string]any{"title": "Page"}},
			"guide.md",
			"Page",
		},
		{
			"heading wins over filename",
			DocumentData{Content: "# Getting Started\n\nBody.\n"},
			"docs/guide.md",
			"Getting Started",
		},
		{
			"filename when there is no heading",
			DocumentData{Content: "Body only.\n"},
			"docs/guide.md",
			"guide",
		},
		{
			"fallback when there is neither",
			DocumentData{},
			"",
			"Site",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := documentTitle(tt.doc, tt.filePath, "Site"); got != tt.want {
				t.Fatalf("documentTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListingTitle(t *testing.T) {
	tests := []struct {
		relDir string
		want   string
	}{
		{"", "Index of /"},
		{".", "Index of /"},
		{"planning", "Index of /planning"},
		{"a/b/", "Index of /a/b"},
	}
	for _, tt := range tests {
		if got := listingTitle(tt.relDir); got != tt.want {
			t.Errorf("listingTitle(%q) = %q, want %q", tt.relDir, got, tt.want)
		}
	}
}
