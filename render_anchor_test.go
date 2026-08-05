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
		html, err := markdownToHTMLWithContext(Config{}, "## "+tt.heading+"\n", "")
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
	html, err := markdownToHTMLWithContext(Config{}, "## Setup\n\n## Setup\n", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="setup"`, `id="setup-1"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
}
