package mdvet

import (
	"strings"
	"testing"
)

func TestAssetsCheck(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "local link ok",
			files: map[string]string{
				"a.md": "[b](b.md)\n",
				"b.md": "# B\n",
			},
		},
		{
			name: "missing local link",
			files: map[string]string{
				"a.md": "[b](missing.md)\n",
			},
			want: []string{"missing.md"},
		},
		{
			name: "missing image",
			files: map[string]string{
				"a.md": "![x](missing.png)\n",
			},
			want: []string{"missing.png"},
		},
		{
			name: "missing media",
			files: map[string]string{
				"a.md": "![demo](demo.mp4)\n",
			},
			want: []string{"media", "demo.mp4"},
		},
		{
			name: "local anchor ok",
			files: map[string]string{
				"a.md": "# Section\n\n[section](#section)\n",
			},
		},
		{
			name: "local anchor missing",
			files: map[string]string{
				"a.md": "# Section\n\n[missing](#missing)\n",
			},
			want: []string{`"missing"`},
		},
		{
			name: "cross anchor missing",
			files: map[string]string{
				"a.md": "[b](b.md#missing)\n",
				"b.md": "# Section\n",
			},
			want: []string{`"missing"`},
		},
		{
			name: "remote skipped",
			files: map[string]string{
				"a.md": "[site](https://example.com/missing.md)\n![x](https://example.com/x.png)\n",
			},
		},
		{
			name: "fenced references skipped",
			files: map[string]string{
				"a.md": "```\n[bad](missing.md)\n![bad](missing.png)\n<div>literal</div>\n```\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := runCheck(t, tt.files, "a.md", AssetsCheck{})
			if len(tt.want) == 0 {
				if len(diags) != 0 {
					t.Fatalf("got %v, want none", diags)
				}
				return
			}
			wantSubstrings(t, diags, tt.want)
		})
	}
}

func TestRunSkipsLegacyAssetChecksWhenAssetsEnabled(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"a.md": "[missing](missing.md)\n",
	})
	diags, err := Run([]string{dir}, AllChecks())
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, d := range diags {
		if strings.Contains(d.Message, "missing.md") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("got %d missing.md diagnostics, want 1: %v", n, diags)
	}
}
