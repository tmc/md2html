package md2html

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelativeRenderedLink(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		target    string
		htmlExt   string
		indexFile string
		want      string
	}{
		{
			name:      "root to nested static page",
			current:   "index.md",
			target:    "guides/getting-started.md",
			htmlExt:   "html",
			indexFile: "index.md",
			want:      "guides/getting-started.html",
		},
		{
			name:      "nested to root static page",
			current:   "guides/getting-started.md",
			target:    "index.md",
			htmlExt:   "html",
			indexFile: "index.md",
			want:      "../index.html",
		},
		{
			name:      "nested to sibling static page",
			current:   "guides/getting-started.md",
			target:    "install.md",
			htmlExt:   "html",
			indexFile: "index.md",
			want:      "../install.html",
		},
		{
			name:      "nested to sibling server page",
			current:   "guides/getting-started.md",
			target:    "install.md",
			indexFile: "index.md",
			want:      "../install",
		},
		{
			name:      "same directory nested page",
			current:   "guides/getting-started.md",
			target:    "guides/index.md",
			htmlExt:   "html",
			indexFile: "index.md",
			want:      "index.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relativeRenderedLink(tt.current, tt.target, tt.htmlExt, tt.indexFile); got != tt.want {
				t.Fatalf("relativeRenderedLink(%q, %q, %q, %q) = %q, want %q",
					tt.current, tt.target, tt.htmlExt, tt.indexFile, got, tt.want)
			}
		})
	}
}

func TestAssetBase(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "./"},
		{name: "top-level file", in: "README.md", want: "./"},
		{name: "one deep", in: "docs/guide.md", want: "../"},
		{name: "multi deep", in: "a/b/c/page.md", want: "../../../"},
		{name: "leading slash", in: "/install.md", want: "./"},
		{name: "leading slash nested", in: "/docs/guide.md", want: "../"},
		{name: "index at root", in: "index.md", want: "./"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assetBase(tt.in); got != tt.want {
				t.Fatalf("assetBase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCanonicalPagePath(t *testing.T) {
	tests := []struct {
		name     string
		rendered string
		htmlExt  string
		want     string
	}{
		{name: "regular page", rendered: "guide/intro.html", htmlExt: "html", want: "guide/intro.html"},
		{name: "root index", rendered: "index.html", htmlExt: "html", want: ""},
		{name: "nested index", rendered: "posts/index.html", htmlExt: "html", want: "posts/"},
		{name: "deep nested index", rendered: "a/b/index.html", htmlExt: "html", want: "a/b/"},
		{name: "index-prefixed page", rendered: "posts/indexing.html", htmlExt: "html", want: "posts/indexing.html"},
		{name: "no extension passthrough", rendered: "guide/intro", htmlExt: "", want: "guide/intro"},
		{name: "dotted extension", rendered: "index.htm", htmlExt: ".htm", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalPagePath(tt.rendered, tt.htmlExt); got != tt.want {
				t.Fatalf("canonicalPagePath(%q, %q) = %q, want %q", tt.rendered, tt.htmlExt, got, tt.want)
			}
		})
	}
}

func TestRewriteLocalMarkdownReference(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		href      string
		htmlExt   string
		indexFile string
		want      string
		ok        bool
	}{
		{
			name:      "rewrites markdown link with fragment",
			current:   "guides/getting-started.md",
			href:      "../install.md#troubleshooting",
			htmlExt:   "html",
			indexFile: "index.md",
			want:      "../install.html#troubleshooting",
			ok:        true,
		},
		{
			name:      "rewrites local readme link",
			current:   "guides/getting-started.md",
			href:      "../index.md",
			htmlExt:   "html",
			indexFile: "index.md",
			want:      "../index.html",
			ok:        true,
		},
		{
			name:    "leaves external link alone",
			current: "guides/getting-started.md",
			href:    "https://example.com/install.md",
			want:    "https://example.com/install.md",
			ok:      false,
		},
		{
			name:    "leaves root absolute link alone",
			current: "guides/getting-started.md",
			href:    "/install.md",
			want:    "/install.md",
			ok:      false,
		},
		{
			name:    "leaves non markdown asset alone",
			current: "guides/getting-started.md",
			href:    "../images/logo.png",
			want:    "../images/logo.png",
			ok:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := rewriteLocalMarkdownReference(tt.current, tt.href, tt.htmlExt, tt.indexFile)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("rewriteLocalMarkdownReference(%q, %q, %q, %q) = (%q, %v), want (%q, %v)",
					tt.current, tt.href, tt.htmlExt, tt.indexFile, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestMarkdownToHTMLWithContextRewritesRelativeLinks(t *testing.T) {
	cfg := Config{HTMLExt: "html", Index: "index.md"}

	html := markdownToHTMLWithContext(cfg, "[Install](../install.md)\n", "guides/getting-started.md")
	if !strings.Contains(html, `href="../install.html"`) {
		t.Fatalf("rendered HTML missing rewritten markdown href:\n%s", html)
	}
	if strings.Contains(html, `href="/install.html"`) {
		t.Fatalf("rendered HTML still contains root-absolute href:\n%s", html)
	}
}

func TestMarkdownToHTMLWithContextRewritesOKFRootLinks(t *testing.T) {
	cfg := Config{Format: "okf", HTMLExt: "html", Index: "index.md"}

	html := markdownToHTMLWithContext(cfg, "[Orders](/tables/orders.md?view=all#columns)\n", "datasets/sales.md")
	if !strings.Contains(html, `href="../tables/orders.html?view=all#columns"`) {
		t.Fatalf("rendered HTML missing rewritten OKF href:\n%s", html)
	}
}

func TestMarkdownToHTMLWithContextLeavesRootLinksInDefaultFormat(t *testing.T) {
	html := markdownToHTMLWithContext(Config{HTMLExt: "html"}, "[Orders](/tables/orders.md)\n", "datasets/sales.md")
	if !strings.Contains(html, `href="/tables/orders.md"`) {
		t.Fatalf("rendered HTML rewrote ordinary root-relative href:\n%s", html)
	}
}

func TestMarkdownToHTMLWithContextRewritesUnsafeHTMLLinks(t *testing.T) {
	cfg := Config{AllowUnsafe: true, HTMLExt: "html", Index: "index.md"}

	html := markdownToHTMLWithContext(cfg, `<div><a href="../install.md#x">Install</a></div>`, "guides/getting-started.md")
	if !strings.Contains(html, `href="../install.html#x"`) {
		t.Fatalf("rendered HTML missing rewritten raw HTML href:\n%s", html)
	}
	if strings.Contains(html, `href="../install.md#x"`) {
		t.Fatalf("rendered HTML still contains markdown source href:\n%s", html)
	}
}

func TestMarkdownToHTMLWithContextRewritesUnsafeOKFRootLinks(t *testing.T) {
	cfg := Config{AllowUnsafe: true, Format: "okf", HTMLExt: "html", Index: "index.md"}

	html := markdownToHTMLWithContext(cfg, `<div><a href="/tables/orders.md#columns">Orders</a></div>`, "datasets/sales.md")
	if !strings.Contains(html, `href="../tables/orders.html#columns"`) {
		t.Fatalf("rendered HTML missing rewritten OKF raw HTML href:\n%s", html)
	}
}

func TestValidateFormat(t *testing.T) {
	for _, format := range []string{"", "okf", " OKF "} {
		if err := validateFormat(format); err != nil {
			t.Errorf("validateFormat(%q) = %v, want nil", format, err)
		}
	}
	if err := validateFormat("openspec"); err == nil {
		t.Fatal("validateFormat(\"openspec\") = nil, want error")
	}
}

func TestRunRejectsInvalidFormat(t *testing.T) {
	err := Run(context.Background(), Config{Format: "openspec"}, slog.New(slog.NewTextHandler(io.Discard, nil)), io.Discard, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid format") {
		t.Fatalf("Run() error = %v, want invalid format", err)
	}
}

func TestGenerateStaticHTMLOKFLinks(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("datasets/sales.md", "---\ntype: Dataset\n---\n[Orders](/tables/orders.md)\n")
	write("tables/orders.md", "---\ntype: Table\n---\n# Orders\n")

	out := filepath.Join(root, "out")
	cfg := Config{Source: root, HTML: out, HTMLExt: "html", Format: "okf"}
	if err := generateStaticHTML(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("generateStaticHTML() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(out, "datasets", "sales.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `href="../tables/orders.html"`) {
		t.Fatalf("static HTML missing rewritten OKF href:\n%s", data)
	}
}

func TestGenerateStaticHTMLSummaryLinksAreRelative(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("SUMMARY.md", `# Summary

- [Home](index.md)
- [Install](install.md)
- [Getting Started](guides/getting-started.md)
`)
	write("index.md", "# Home\n")
	write("install.md", "# Install\n")
	write("guides/getting-started.md", "# Getting Started\n")

	out := filepath.Join(root, "out")
	cfg := Config{
		Source:  root,
		HTML:    out,
		Index:   "index.md",
		HTMLExt: "html",
		Nav:     true,
	}

	if err := generateStaticHTML(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("generateStaticHTML() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(out, "guides", "getting-started.html"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	html := string(data)
	if !strings.Contains(html, `href="../install.html"`) {
		t.Fatalf("sidebar did not render relative install href:\n%s", html)
	}
	if strings.Contains(html, `href="install.html"`) {
		t.Fatalf("sidebar still contains nested-page broken href:\n%s", html)
	}
	if !strings.Contains(html, `href="../index.html"`) {
		t.Fatalf("sidebar did not render relative home href:\n%s", html)
	}
}
