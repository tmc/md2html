package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteRootLink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "docs.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []markdownFile{
		{RelPath: "docs/bar.md"},
		{RelPath: "docs/foo.md"},
		{RelPath: "docs/sub/x.md"},
		{RelPath: "docs/guide/index.md"},
		{RelPath: "README.md"},
	}
	site := &preparedSite{
		config: Config{HTMLExt: "html", Index: "index.md"},
		links:  newSiteLinks(root, files),
	}

	tests := []struct {
		name    string
		current string
		href    string
		want    string
	}{
		{"sibling", "docs/bar.md", "/docs/foo", "foo.html"},
		{"parent", "docs/sub/x.md", "/docs/foo", "../foo.html"},
		{"fragment", "docs/bar.md", "/docs/foo#h", "foo.html#h"},
		{"query", "docs/bar.md", "/docs/foo?x=1#h", "foo.html?x=1#h"},
		{"source name", "docs/bar.md", "/docs/foo.md", "foo.html"},
		{"rendered name", "docs/bar.md", "/docs/foo.html", "foo.html"},
		{"trailing slash", "docs/bar.md", "/docs/guide/", "guide/index.html"},
		{"directory index", "docs/sub/x.md", "/docs/guide", "../guide/index.html"},
		{"site root", "docs/sub/x.md", "/", "../../README.html"},
		{"missing", "docs/bar.md", "/docs/missing", "/docs/missing"},
		{"asset", "docs/bar.md", "/images/logo.png", "/images/logo.png"},
		{"external", "docs/bar.md", "https://example.com/docs/foo", "https://example.com/docs/foo"},
		{"protocol relative", "docs/bar.md", "//example.com/docs/foo", "//example.com/docs/foo"},
		{"fragment only", "docs/bar.md", "#frag", "#frag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := site.rewriteLink(tt.current, tt.href)
			if !ok {
				got = tt.href
			}
			if got != tt.want {
				t.Errorf("rewriteLink(%q, %q) = %q, want %q", tt.current, tt.href, got, tt.want)
			}
		})
	}
}

func TestRewriteRootLinkBelowSiteRoot(t *testing.T) {
	// Building docs/ alone: links still name pages by their path below
	// the docs.json directory.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "docs.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := &preparedSite{
		config: Config{HTMLExt: "html", Index: "index.md"},
		links:  newSiteLinks(filepath.Join(root, "docs"), []markdownFile{{RelPath: "bar.md"}, {RelPath: "sub/foo.md"}}),
	}
	got, ok := site.rewriteLink("bar.md", "/docs/sub/foo#h")
	if want := "sub/foo.html#h"; !ok || got != want {
		t.Errorf("rewriteLink = %q, %v, want %q", got, ok, want)
	}
}

func TestMarkdownToHTMLRewritesRootLinks(t *testing.T) {
	site := &preparedSite{
		config: Config{AllowUnsafe: true, HTMLExt: "html", Index: "index.md"},
		links:  newSiteLinks(t.TempDir(), []markdownFile{{RelPath: "docs/foo.md"}}),
	}
	md := "[Foo](/docs/foo#h)\n\n<div><a href=\"/docs/foo\">Foo</a></div>\n\n[Gone](/docs/gone)\n"
	html, err := site.markdownToHTML(md, "docs/sub/x.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="../foo.html#h"`, `href="../foo.html"`, `href="/docs/gone"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}
