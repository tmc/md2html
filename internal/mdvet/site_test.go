package mdvet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeBase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"/", ""},
		{"/docs", "/docs"},
		{"docs", "/docs"},
		{"/docs/", "/docs"},
		{"/docs//", "/docs"},
		{"  /docs  ", "/docs"},
	}
	for _, tt := range tests {
		if got := normalizeBase(tt.in); got != tt.want {
			t.Errorf("normalizeBase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestSiteResolve covers the mapping from a rendered URL back to the
// source file behind it.
func TestSiteResolve(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"index.md",
		"quickstart.md",
		"churl.markdown",
		"guide/index.md",
		"deep/nested/page.md",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		site     Site
		dest     string
		wantRel  string // "" means it must not resolve
		wantFrag string
	}{
		{"extensionless under base", Site{Base: "/docs", Root: root}, "/docs/quickstart", "quickstart.md", ""},
		{"fragment carried through", Site{Base: "/docs", Root: root}, "/docs/churl#exit-status", "churl.markdown", "exit-status"},
		{"base itself is the index", Site{Base: "/docs", Root: root}, "/docs", "index.md", ""},
		{"trailing slash is the index", Site{Base: "/docs", Root: root}, "/docs/", "index.md", ""},
		{"directory index", Site{Base: "/docs", Root: root}, "/docs/guide", "guide/index.md", ""},
		{"nested path", Site{Base: "/docs", Root: root}, "/docs/deep/nested/page", "deep/nested/page.md", ""},
		{"explicit extension", Site{Base: "/docs", Root: root}, "/docs/quickstart.md", "quickstart.md", ""},
		{"percent-encoded", Site{Base: "/docs", Root: root}, "/docs/quick%73tart", "quickstart.md", ""},

		{"no base configured", Site{Root: root}, "/quickstart", "quickstart.md", ""},
		{"outside the prefix", Site{Base: "/docs", Root: root}, "/blog/post", "", ""},
		{"prefix is not a path boundary", Site{Base: "/docs", Root: root}, "/docsite/x", "", ""},
		{"no such page", Site{Base: "/docs", Root: root}, "/docs/missing", "", ""},
		{"relative link is not ours", Site{Base: "/docs", Root: root}, "quickstart.md", "", ""},
		{"external URL", Site{Base: "/docs", Root: root}, "https://example.com/docs/quickstart", "", ""},
		{"no root disables resolution", Site{Base: "/docs"}, "/docs/quickstart", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, frag, ok := tt.site.resolve(tt.dest)
			if tt.wantRel == "" {
				if ok {
					t.Fatalf("resolve(%q) = %q, want no resolution", tt.dest, file)
				}
				return
			}
			if !ok {
				t.Fatalf("resolve(%q) did not resolve, want %q", tt.dest, tt.wantRel)
			}
			want := filepath.Join(root, filepath.FromSlash(tt.wantRel))
			if file != want {
				t.Errorf("resolve(%q) = %q, want %q", tt.dest, file, want)
			}
			if frag != tt.wantFrag {
				t.Errorf("resolve(%q) fragment = %q, want %q", tt.dest, frag, tt.wantFrag)
			}
		})
	}
}

func TestSiteOwns(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		site Site
		dest string
		want bool
	}{
		{Site{Base: "/docs", Root: root}, "/docs/anything", true},
		{Site{Base: "/docs", Root: root}, "/docs", true},
		{Site{Base: "/docs", Root: root}, "/blog/post", false},
		{Site{Base: "/docs", Root: root}, "/docsite", false},
		{Site{Root: root}, "/anything", true},
		{Site{Root: root}, "relative.md", false},
		{Site{Root: root}, "https://example.com/x", false},
		{Site{Base: "/docs"}, "/docs/x", false}, // no root
	}
	for _, tt := range tests {
		if got := tt.site.owns(tt.dest); got != tt.want {
			t.Errorf("Site{Base:%q}.owns(%q) = %v, want %v", tt.site.Base, tt.dest, got, tt.want)
		}
	}
}

// TestAnchorCheckResolvesRenderedURLs is the point of the whole thing:
// docs written for a hosted site link by rendered URL, and those
// anchors went unchecked.
func TestAnchorCheckResolvesRenderedURLs(t *testing.T) {
	files := map[string]string{
		"quickstart.md": "# Quickstart\n\nSee [ok](/docs/churl#exit-status).\n\nSee [bad](/docs/churl#no-such-thing).\n",
		"churl.md":      "# churl\n\n## Exit status\n\ntext\n",
	}

	t.Run("without a site the anchor is unchecked", func(t *testing.T) {
		diags := runCheck(t, files, "quickstart.md", AnchorCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none — nothing can resolve /docs/churl", diags)
		}
	})

	t.Run("with a site the bad anchor is caught", func(t *testing.T) {
		diags := runCheckSite(t, files, "quickstart.md", AnchorCheck{}, Site{Base: "/docs"})
		if len(diags) != 1 {
			t.Fatalf("got %v, want exactly the one bad anchor", diags)
		}
		wantSubstrings(t, diags, []string{"no-such-thing"})
	})
}

// TestLinkCheckResolvesRenderedURLs checks the other half: a rooted
// link inside the tree's own prefix is a page URL, and one that names
// no page is broken rather than unverifiable.
func TestLinkCheckResolvesRenderedURLs(t *testing.T) {
	files := map[string]string{
		"quickstart.md": "# Quickstart\n\n[ok](/docs/churl)\n\n[gone](/docs/nope)\n\n[elsewhere](/blog/post)\n",
		"churl.md":      "# churl\n",
	}

	diags := runCheckSite(t, files, "quickstart.md", LinkCheck{}, Site{Base: "/docs"})
	if len(diags) != 2 {
		t.Fatalf("got %v, want two: the missing page and the off-site absolute path", diags)
	}
	wantSubstrings(t, diags, []string{
		"no page in this tree renders that URL",
		"absolute path; mdvet refuses to validate",
	})
}
