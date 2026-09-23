package md2html

import (
	"strings"
	"testing"
)

// TestRenderHeadingIDs pins the ids the rendered page hands out. They
// are the URLs published deep links use, and they have to match what
// mdvet validates and what Mintlify produces for the same heading.
func TestRenderHeadingIDs(t *testing.T) {
	tests := []struct {
		heading string
		want    string
	}{
		{"Exit status", "exit-status"},
		{"click/type_text/hover", "click/type_text/hover"},
		{"cdp — generalized browser scripting", "cdp-—-generalized-browser-scripting"},
		{"extension_console/extension_evaluate", "extension_console/extension_evaluate"},
	}
	for _, tt := range tests {
		html, err := markdownToHTML(Config{}, "## "+tt.heading+"\n", "")
		if err != nil {
			t.Fatal(err)
		}
		want := `id="` + tt.want + `"`
		if !strings.Contains(html, want) {
			t.Errorf("heading %q rendered as:\n%s\nwant %s", tt.heading, html, want)
		}
	}
}

// TestRenderHeadingIDsDedupe checks that a repeated heading still gets
// a distinct anchor rather than two elements sharing an id.
func TestRenderHeadingIDsDedupe(t *testing.T) {
	html, err := markdownToHTML(Config{}, "## Setup\n\n## Setup\n", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="setup"`, `id="setup-1"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}

// TestRenderTypography pins the smart-punctuation rules against the
// renderer that publishes these docs. goldmark's defaults disagree with
// it about dashes, so both directions are checked here rather than
// assumed.
func TestRenderTypography(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
		not  []string
	}{
		{
			name: "two hyphens become an em dash in text and id",
			in:   "## a -- b\n",
			want: []string{`id="a-—-b"`, "a &mdash; b"},
		},
		{
			name: "three hyphens are left alone",
			in:   "## a --- b\n",
			want: []string{`id="a-b"`, "a --- b"},
			not:  []string{"&mdash;"},
		},
		{
			name: "apostrophes and quotes curl in prose",
			in:   "the tools' \"docs\"\n",
			want: []string{"&rsquo;", "&ldquo;", "&rdquo;"},
		},
		{
			name: "nothing inside a code span is touched",
			in:   "use `--flag` and `it's` and `\"x\"`\n",
			not:  []string{"&mdash;", "&rsquo;", "&ldquo;"},
		},
		{
			name: "nothing inside a fence is touched",
			in:   "```\nrun -- 'this' \"raw\"\n```\n",
			not:  []string{"&mdash;", "&rsquo;", "&ldquo;"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := markdownToHTML(Config{}, tt.in, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q in:\n%s", w, html)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(html, n) {
					t.Errorf("unexpected %q in:\n%s", n, html)
				}
			}
		})
	}
}
