package mdvet

import (
	"strings"
	"testing"
)

func TestRawHTMLCheck(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // substrings, one per expected diagnostic
	}{
		{
			name: "generic type in prose is dropped",
			src:  "Returns a List<String> of names.\n",
			want: []string{"<String>"},
		},
		{
			name: "inline tag is dropped",
			src:  "One<br>two\n",
			want: []string{"<br>"},
		},
		{
			name: "html block reported once",
			src:  "<div class=\"x\">\n  <span>hi</span>\n</div>\n",
			want: []string{"<div"},
		},
		{
			name: "code span is not html",
			src:  "Returns a `List<String>` of names.\n",
			want: nil,
		},
		{
			name: "fenced block is not html",
			src:  "```go\nvar x []String\n```\n",
			want: nil,
		},
		{
			name: "autolink is not raw html",
			src:  "See <https://example.com> for more.\n",
			want: nil,
		},
		{
			name: "plain prose",
			src:  "# Title\n\nNothing angled here.\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := runCheck(t, map[string]string{"a.md": tt.src}, "a.md", RawHTMLCheck{})
			if len(diags) != len(tt.want) {
				t.Fatalf("got %d diagnostics %v, want %d", len(diags), diags, len(tt.want))
			}
			for i, want := range tt.want {
				if !strings.Contains(diags[i].Message, want) {
					t.Errorf("diagnostic %d = %q, want it to mention %q", i, diags[i].Message, want)
				}
				if diags[i].Check != "raw-html" {
					t.Errorf("check = %q, want %q", diags[i].Check, "raw-html")
				}
			}
		})
	}
}

// TestRawHTMLCheckLeavesComponentsToComponentCheck keeps the two checks
// from reporting the same line twice.
func TestRawHTMLCheckLeavesComponentsToComponentCheck(t *testing.T) {
	diags := runCheck(t, map[string]string{
		"a.md": "<Warning>\nCareful.\n</Warning>\n\nInline <Badge>x</Badge> too.\n",
	}, "a.md", RawHTMLCheck{})
	if len(diags) != 0 {
		t.Errorf("got %v, want none: component tags belong to ComponentCheck", diags)
	}
}
