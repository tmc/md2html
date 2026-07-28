package md2html

import (
	"strings"
	"testing"
)

func TestGitHubAlerts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
		not  []string
	}{
		{
			name: "basic alerts",
			in: strings.Join([]string{
				"> [!NOTE]",
				"> note",
				"",
				"> [!TIP]",
				"> tip",
				"",
				"> [!IMPORTANT]",
				"> important",
				"",
				"> [!WARNING]",
				"> warning",
				"",
				"> [!CAUTION]",
				"> caution",
				"",
			}, "\n"),
			want: []string{
				`<div class="admonition adm-note">`,
				`<div class="admonition adm-tip">`,
				`<div class="admonition adm-important">`,
				`<div class="admonition adm-warning">`,
				`<div class="admonition adm-danger">`,
				`<p>note</p>`,
				`<p>caution</p>`,
			},
			not: []string{"[!NOTE]", "[!CAUTION]", "<blockquote>"},
		},
		{
			name: "marker in fenced code",
			in: strings.Join([]string{
				"```md",
				"> [!NOTE]",
				"> not an alert",
				"```",
				"",
			}, "\n"),
			want: []string{"[!NOTE]", "not an alert"},
			not:  []string{`class="admonition adm-note"`},
		},
		{
			name: "html literal in fenced code",
			in: strings.Join([]string{
				"```html",
				"<div>literal</div>",
				"```",
				"",
			}, "\n"),
			want: []string{"&lt;", "div", "literal", "&lt;/"},
			not:  []string{`<div>literal</div>`},
		},
		{
			name: "multi paragraph body",
			in: strings.Join([]string{
				"> [!WARNING]",
				"> first",
				">",
				"> second",
				"",
			}, "\n"),
			want: []string{
				`<div class="admonition adm-warning">`,
				"<p>first</p>",
				"<p>second</p>",
			},
			not: []string{"[!WARNING]"},
		},
		{
			name: "nested formatting",
			in: strings.Join([]string{
				"> [!TIP]",
				"> Use **bold** and `code` with [a link](https://example.com).",
				"",
			}, "\n"),
			want: []string{
				`<div class="admonition adm-tip">`,
				"<strong>bold</strong>",
				"<code>code</code>",
				`<a href="https://example.com">a link</a>`,
			},
			not: []string{"[!TIP]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustRenderMarkdown(t, Config{}, tt.in, "")
			for _, s := range tt.want {
				if !strings.Contains(got, s) {
					t.Fatalf("markdownToHTML() missing %q in:\n%s", s, got)
				}
			}
			for _, s := range tt.not {
				if strings.Contains(got, s) {
					t.Fatalf("markdownToHTML() unexpectedly contains %q in:\n%s", s, got)
				}
			}
		})
	}
}
