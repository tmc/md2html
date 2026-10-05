package mdvet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/md2html/internal/markdown/include"
)

func TestIncludeCheck(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"examples/main.go":    "package main\n\n// region run\nfunc main() {}\n// endregion run\n",
		"docs/run.sh":         "echo hi\n",
		"docs/private/key.go": "package private\n",
		".mintignore":         "private/\n",
		"docs/page.md": "# Page\n\n" +
			"<Code src=\"run.sh\" />\n\n" +
			"<Code src=\"gone.sh\" />\n\n" +
			"<Code src=\"/examples/main.go\" region=\"run\" link />\n\n" +
			"<Code src=\"/examples/main.go\" region=\"walk\" />\n\n" +
			"<Code src=\"private/key.go\" />\n\n" +
			"```mdx\n<Code src=\"example.go\" />\n```\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	page := filepath.Join(root, "docs", "page.md")
	type diag struct {
		line   int
		substr string
	}
	tests := []struct {
		name  string
		check IncludeCheck
		site  Site
		want  []diag
	}{
		{
			// Without a site the include root is the page's directory.
			"page directory",
			IncludeCheck{},
			Site{},
			[]diag{{5, "gone.sh: no such file"}, {7, "examples/main.go: no such file"}, {9, "examples/main.go: no such file"}, {11, "excluded by the ignore file"}},
		},
		{
			"site root",
			IncludeCheck{},
			Site{Root: root},
			[]diag{{5, "docs/gone.sh: no such file"}, {9, "no region walk"}, {11, "docs/private/key.go: excluded by the ignore file"}},
		},
		{
			"include root",
			IncludeCheck{Root: filepath.Join(root, "docs")},
			Site{Root: root},
			[]diag{{5, "gone.sh: no such file"}, {7, "examples/main.go: no such file"}, {9, "examples/main.go: no such file"}, {11, "private/key.go: excluded by the ignore file"}},
		},
		{
			// A resolver with no way to link reports a tag that asks,
			// and one with no ignore file excludes nothing.
			"resolver",
			IncludeCheck{Resolver: &include.Resolver{Root: root}},
			Site{},
			[]diag{{5, "docs/gone.sh: no such file"}, {7, "link needs a repository"}, {9, "no region walk"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The components check runs too: a Code tag is not an
			// unknown component.
			diags, err := RunSite([]string{page}, []Check{ComponentCheck{}, tt.check}, tt.site)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) != len(tt.want) {
				t.Fatalf("got %d diagnostics, want %d: %v", len(diags), len(tt.want), diags)
			}
			for i, w := range tt.want {
				d := diags[i]
				if d.Check != "includes" || d.Line != w.line || !strings.Contains(d.Message, w.substr) {
					t.Errorf("diagnostic %d = %v, want includes on line %d with %q", i, d, w.line, w.substr)
				}
			}
		})
	}
}
