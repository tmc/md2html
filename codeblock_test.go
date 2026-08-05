package md2html

import (
	"strings"
	"testing"
)

func TestCodeLanguageLabel(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"go", "Go"},
		{"Go", "Go"},
		{"js", "JavaScript"},
		{"cpp", "C++"},
		{"zig", "ZIG"},
		{"", ""},
		{"text", ""},
		{"plaintext", ""},
	}
	for _, tt := range tests {
		if got := codeLanguageLabel(tt.lang); got != tt.want {
			t.Errorf("codeLanguageLabel(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

// TestRenderCodeBlockLanguage checks that a fenced block reaches the page
// wrapped in a container naming its language, and that a fence with no
// language stays unlabelled rather than gaining an empty strip.
func TestRenderCodeBlockLanguage(t *testing.T) {
	cfg := Config{}
	html, err := markdownToHTMLWithContext(cfg, "```go\nfmt.Println()\n```\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<div class="md-code" data-language="Go">`) {
		t.Errorf("labelled wrapper missing from:\n%s", html)
	}
	if !strings.Contains(html, `<span class="md-code-lang" aria-hidden="true">Go</span>`) {
		t.Errorf("language strip missing from:\n%s", html)
	}

	plain, err := markdownToHTMLWithContext(cfg, "```\nno language\n```\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain, `<div class="md-code">`) {
		t.Errorf("wrapper missing from:\n%s", plain)
	}
	if strings.Contains(plain, "md-code-lang") {
		t.Errorf("unlabelled fence gained a language strip:\n%s", plain)
	}
	if !strings.Contains(plain, "no language") {
		t.Errorf("code text lost:\n%s", plain)
	}
}
