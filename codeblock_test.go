package md2html

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
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
	html, err := markdownToHTML(cfg, "```go\nfmt.Println()\n```\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<div class="md-code" data-language="Go">`) {
		t.Errorf("labelled wrapper missing from:\n%s", html)
	}
	if !strings.Contains(html, `<span class="md-code-lang" aria-hidden="true">Go</span>`) {
		t.Errorf("language strip missing from:\n%s", html)
	}

	plain, err := markdownToHTML(cfg, "```\nno language\n```\n", "")
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

func TestCodeBlockTitle(t *testing.T) {
	tests := []struct {
		name string
		info string
		want string // "" means no caption
	}{
		{"none", "go", ""},
		{"title", `go {title="main.go"}`, `<div class="md-code-title">main.go</div>`},
		{"escaped", `go {title="a <b> & c"}`, `<div class="md-code-title">a &lt;b&gt; &amp; c</div>`},
		{"link", `go {title="main.go" href="https://example.com/main.go#L3"}`, `<div class="md-code-title"><a href="https://example.com/main.go#L3">main.go</a></div>`},
		{"relative link", `go {title="main.go" href="../main.go"}`, `<div class="md-code-title"><a href="../main.go">main.go</a></div>`},
		{"script link", `go {title="main.go" href="javascript:alert(1)"}`, `<div class="md-code-title">main.go</div>`},
		{"link without title", `go {href="https://example.com"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := markdownToHTML(Config{}, "```"+tt.info+"\nx := 1\n```\n", "")
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if strings.Contains(html, "md-code-title") {
					t.Errorf("unexpected caption:\n%s", html)
				}
				return
			}
			if !strings.Contains(html, tt.want) {
				t.Errorf("missing %s in:\n%s", tt.want, html)
			}
		})
	}
}

func TestCodeBlockTitleLinkRewritten(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		href string
		want string
	}{
		{"page", Config{HTMLExt: "html"}, "other.md#x", `href="other.html#x"`},
		{"page in dir layout", Config{HTMLExt: "html", HTMLLayout: "dir"}, "other.md", `href="../other/"`},
		{"file in dir layout", Config{HTMLExt: "html", HTMLLayout: "dir"}, "main.go", `href="../main.go"`},
		{"web address", Config{HTMLExt: "html", HTMLLayout: "dir"}, "https://example.com/a.md", `href="https://example.com/a.md"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "```go {title=\"main.go\" href=\"" + tt.href + "\"}\nx := 1\n```\n"
			html, err := markdownToHTML(tt.cfg, src, "page.md")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(html, tt.want) {
				t.Errorf("missing %s in:\n%s", tt.want, html)
			}
		})
	}
}

// A fence's own attributes survive an extension setting others on the
// block, as -jsonspec does.
func TestCodeBlockTitleWithJSONSpec(t *testing.T) {
	dir := t.TempDir()
	config := `{"prefixes":["acme/"],"badge_url":"schemas.html#%s","badge_label":"%s schema"}`
	if err := os.WriteFile(filepath.Join(dir, "jsonspec.json"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := prepareJSONSpec(Config{JSONSpec: dir}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	src := "```json {title=\"order.json\"}\n{\"type\": \"acme/order\"}\n```\n"
	html, err := s.markdownToHTML(src, "page.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<div class="md-code-title">order.json</div>`, "order schema"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}
