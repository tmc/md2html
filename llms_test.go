package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLLMSSummary(t *testing.T) {
	tests := []struct {
		name string
		doc  DocumentData
		want string
	}{
		{
			name: "description",
			doc: DocumentData{
				Frontmatter: map[string]interface{}{"description": "frontmatter summary"},
				Content:     "# Title\n\nBody paragraph.",
			},
			want: "frontmatter summary",
		},
		{
			name: "first paragraph",
			doc: DocumentData{
				Frontmatter: map[string]interface{}{},
				Content:     "# Title\n\nFirst **paragraph** with [a link](guide.md).\n\nSecond paragraph.",
			},
			want: "First paragraph with a link.",
		},
		{
			name: "truncate",
			doc: DocumentData{
				Frontmatter: map[string]interface{}{},
				Content:     strings.Repeat("word ", 80),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llmsSummary(tt.doc)
			if tt.name == "truncate" {
				if len([]rune(got)) > 163 || !strings.HasSuffix(got, "...") {
					t.Fatalf("llmsSummary() = %q, want truncated string", got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("llmsSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteLLMSFullSplits(t *testing.T) {
	dir := t.TempDir()
	pages := []llmsPage{
		{URL: "a.html", Content: strings.Repeat("a", 40)},
		{URL: "b.html", Content: strings.Repeat("b", 40)},
		{URL: "c.html", Content: strings.Repeat("c", 40)},
	}

	if err := writeLLMSFull(dir, pages, 70); err != nil {
		t.Fatalf("writeLLMSFull() error = %v", err)
	}

	for _, name := range []string{"llms-full-001.txt", "llms-full-002.txt", "llms-full-003.txt"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		if !strings.HasPrefix(string(data), "llms-full part ") {
			t.Fatalf("%s missing manifest header:\n%s", name, data)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "llms-full.txt")); !os.IsNotExist(err) {
		t.Fatalf("unsplit llms-full.txt exists after split")
	}
}
