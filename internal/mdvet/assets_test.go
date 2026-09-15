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

// AssetsCheck supersedes LinkCheck in a default run, so it has to
// resolve site-rooted URLs the same way: without this it reported every
// "/docs/page" link in a -base tree as an unvalidatable absolute path.
func TestAssetsCheckResolvesSiteURLs(t *testing.T) {
	t.Run("resolvable URL is silent", func(t *testing.T) {
		diags := runCheckSite(t, map[string]string{
			"a.md":     "[guide](/docs/guide)\n",
			"guide.md": "# Guide\n\n## Setup\n",
		}, "a.md", AssetsCheck{}, Site{Base: "/docs"})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	t.Run("unresolvable URL is reported", func(t *testing.T) {
		diags := runCheckSite(t, map[string]string{
			"a.md": "[gone](/docs/missing)\n",
		}, "a.md", AssetsCheck{}, Site{Base: "/docs"})
		wantSubstrings(t, diags, []string{"no page in this tree renders that URL"})
	})
	t.Run("anchor in a site URL is checked", func(t *testing.T) {
		diags := runCheckSite(t, map[string]string{
			"a.md":     "[setup](/docs/guide#nope)\n",
			"guide.md": "# Guide\n\n## Setup\n",
		}, "a.md", AssetsCheck{}, Site{Base: "/docs"})
		wantSubstrings(t, diags, []string{`has no heading with id "nope"`})
	})
	t.Run("URL outside the prefix is still absolute", func(t *testing.T) {
		diags := runCheckSite(t, map[string]string{
			"a.md": "[x](/elsewhere/page)\n",
		}, "a.md", AssetsCheck{}, Site{Base: "/docs"})
		wantSubstrings(t, diags, []string{"absolute path"})
	})
}
