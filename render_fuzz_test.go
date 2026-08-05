package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzRenderFragment(f *testing.F) {
	for _, seed := range renderSeeds(f) {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, markdown string) {
		if len(markdown) > 64<<10 {
			t.Skip()
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic: %v\ninput=%q", r, markdown)
			}
		}()
		_, _ = RenderFragment(markdown, "docs/page.md", FragmentOptions{HTMLExt: ".html"})
		_, _ = RenderFragment(markdown, "docs/page.md", FragmentOptions{
			AllowUnsafe: true,
			TOC:         true,
			HTMLExt:     ".html",
			Frontmatter: map[string]any{"mermaid_theme": "auto"},
		})
	})
}

func renderSeeds(t *testing.F) []string {
	seeds := []string{
		"",
		"# Title\n\nHello [x](other.md#anchor)\n",
		"> [!NOTE]\n> note\n",
		"```go\nfunc main() {}\n```\n",
		"::: tabs os\n::: tab macOS\n```sh\necho hi\n```\n:::\n:::\n",
		"<div>\n# inside\n</div>\n",
		strings.Repeat("> ", 64) + "[!WARNING]\ndeep\n",
		"```\nunterminated\n",
		"```md\n> [!NOTE]\n> not an alert\n```\n",
		"---\ntitle: [unterminated\n---\n# body\n",
		"nul\x00byte\n\n[bad](%zz)\n",
	}
	for _, name := range []string{"README.md"} {
		if b, err := os.ReadFile(name); err == nil {
			seeds = append(seeds, string(b))
		}
	}
	err := filepath.WalkDir("testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "_md2html" {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".markdown":
		default:
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil || len(b) > 64<<10 {
			return err
		}
		seeds = append(seeds, string(b))
		return nil
	})
	if err != nil {
		t.Fatalf("walk testdata: %v", err)
	}
	return seeds
}
