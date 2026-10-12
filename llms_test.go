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
				Frontmatter: map[string]any{"description": "frontmatter summary"},
				Content:     "# Title\n\nBody paragraph.",
			},
			want: "frontmatter summary",
		},
		{
			name: "first paragraph",
			doc: DocumentData{
				Frontmatter: map[string]any{},
				Content:     "# Title\n\nFirst **paragraph** with [a link](guide.md).\n\nSecond paragraph.",
			},
			want: "First paragraph with a link.",
		},
		{
			name: "component",
			doc: DocumentData{
				Content: "# Agents\n\nimport Tabs from '@theme/Tabs';\n\n<Info>\nConceptual guide to agents.\n</Info>\n\nMore.",
			},
			want: "Conceptual guide to agents.",
		},
		{
			name: "inline tags",
			doc: DocumentData{
				Content: "<Info> Conceptual <b>guide</b> to agents, see <https://go.dev>. </Info>\n",
			},
			want: "Conceptual guide to agents, see <https://go.dev>.",
		},
		{
			name: "markup only",
			doc: DocumentData{
				Content: "# Title\n\n<Frame>\n\n![shot](shot.png)\n\n</Frame>\n\n```go\n// not prose\n```\n\nThe prose.\n",
			},
			want: "The prose.",
		},
		{
			name: "longer fence",
			doc: DocumentData{
				Content: "````md\n```go\nx\n```\nstill code\n````\n\nThe prose.\n",
			},
			want: "The prose.",
		},
		{
			name: "tag over several lines",
			doc: DocumentData{
				Content: "<Card\n  title=\"Start\"\n  href=\"/start\"\n>\nBegin here.\n</Card>\n",
			},
			want: "Begin here.",
		},
		{
			name: "code span",
			doc: DocumentData{
				Content: "Returns a `List<T>` of <b>items</b>.\n",
			},
			want: "Returns a List<T> of items.",
		},
		{
			name: "no prose",
			doc: DocumentData{
				Content: "# Title\n\n<DocCardList />\n",
			},
			want: "",
		},
		{
			name: "truncate",
			doc: DocumentData{
				Frontmatter: map[string]any{},
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

func TestFirstHeading(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"atx", "# Title\n\nBody.", "Title"},
		{"atx closed", "# Title #\n", "Title"},
		{"after prose", "Intro.\n\n# Title\n", "Title"},
		{"setext", "Title\n=====\n\nBody.", "Title"},
		{"markup", "# The `Run` **function**\n", "The Run function"},
		{"level 2 only", "## Section\n", ""},
		{"comment in fence", "```bash\n# install the tool\n```\n\n# Title\n", "Title"},
		{"comment in tilde fence", "~~~\n# not a heading\n~~~\n", ""},
		{"empty heading", "#\n\n# Title\n", "Title"},
		{"none", "Body.", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstHeading(tt.in); got != tt.want {
				t.Errorf("firstHeading(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
