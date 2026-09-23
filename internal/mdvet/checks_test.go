package mdvet

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// runCheck builds a tree under a temp dir and runs the named check on
// the named file, returning the diagnostics. files maps relative paths
// to contents.
func runCheck(t *testing.T, files map[string]string, target string, check Check) []Diagnostic {
	t.Helper()
	return runCheckSite(t, files, target, check, Site{})
}

// runCheckSite is runCheck with a site mapping, for the checks that
// resolve links written as rendered URLs. Site.Root is filled in with
// the temporary directory the files were written to.
func runCheckSite(t *testing.T, files map[string]string, target string, check Check, site Site) []Diagnostic {
	t.Helper()
	dir := t.TempDir()
	// Sort to make creation order deterministic on case-insensitive
	// filesystems (so the directory entry casing is whichever entry
	// sorts first, not whichever map iteration produced first).
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, rel := range keys {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(files[rel]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(dir, target)
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if site.Root == "" && site.Base != "" {
		site.Root = dir
	}
	doc := &Document{File: file, Source: src, Tree: parseTree(src), env: newEnv(site)}
	diags, err := check.Check(doc)
	if err != nil {
		t.Fatal(err)
	}
	return diags
}

func wantSubstrings(t *testing.T, diags []Diagnostic, wants []string) {
	t.Helper()
	for _, w := range wants {
		found := false
		for _, d := range diags {
			if strings.Contains(d.Message, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no diagnostic mentions %q; got %v", w, diags)
		}
	}
}

func TestImageCheck(t *testing.T) {
	t.Run("missing image", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "![alt](missing.png)\n",
		}, "a.md", ImageCheck{})
		wantSubstrings(t, diags, []string{"missing.png"})
	})
	t.Run("present image ok", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md":    "![alt](pic.png)\n",
			"pic.png": "x",
		}, "a.md", ImageCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	t.Run("link is ignored", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "[ref](missing.md)\n",
		}, "a.md", ImageCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none: broken link should be LinkCheck's", diags)
		}
	})
	t.Run("absolute path flagged", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "![alt](/Users/me/pic.png)\n",
		}, "a.md", ImageCheck{})
		wantSubstrings(t, diags, []string{"absolute path"})
	})
}

func TestAnchorCheck_AbsolutePathSilent(t *testing.T) {
	// AnchorCheck must not emit anything for an absolute-path link;
	// LinkCheck handles the diagnostic.
	diags := runCheck(t, map[string]string{
		"a.md": "See [there](/Users/me/notes.md#section).\n",
	}, "a.md", AnchorCheck{})
	if len(diags) != 0 {
		t.Errorf("got %v, want none: absolute paths belong to links check", diags)
	}
}

func TestCaseCheck_AbsolutePathSilent(t *testing.T) {
	diags := runCheck(t, map[string]string{
		"a.md": "[x](/Users/me/notes.md)\n",
	}, "a.md", CaseCheck{})
	if len(diags) != 0 {
		t.Errorf("got %v, want none: absolute paths belong to links check", diags)
	}
}

func TestAnchorCheck(t *testing.T) {
	t.Run("local anchor matches heading", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "# Top\n\n## Sub Section\n\nSee [sub](#sub-section).\n",
		}, "a.md", AnchorCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	t.Run("local anchor missing", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "# Top\n\nSee [nope](#nope).\n",
		}, "a.md", AnchorCheck{})
		wantSubstrings(t, diags, []string{`"nope"`})
	})
	t.Run("cross-file anchor matches", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "See [there](b.md#hello).\n",
			"b.md": "# Hello\n",
		}, "a.md", AnchorCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	t.Run("cross-file anchor missing", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "See [there](b.md#nope).\n",
			"b.md": "# Hello\n",
		}, "a.md", AnchorCheck{})
		wantSubstrings(t, diags, []string{`"nope"`})
	})
	t.Run("anchor on non-markdown skipped", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md":      "See [there](page.html#nope).\n",
			"page.html": "x",
		}, "a.md", AnchorCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
}

func TestDuplicateAnchorCheck(t *testing.T) {
	diags := runCheck(t, map[string]string{
		"a.md": "# Section\n\n## Foo\n\n## Foo\n",
	}, "a.md", DuplicateAnchorCheck{})
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %v", len(diags), diags)
	}
	wantSubstrings(t, diags, []string{"foo"})
}

func TestReferenceDefCheck(t *testing.T) {
	t.Run("undefined reference", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "See [text][missing].\n",
		}, "a.md", ReferenceDefCheck{})
		wantSubstrings(t, diags, []string{`"missing"`})
	})
	t.Run("defined reference ok", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "See [text][ok].\n\n[ok]: https://example.com\n",
		}, "a.md", ReferenceDefCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	t.Run("orphan definition", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "Body.\n\n[orphan]: https://example.com\n",
		}, "a.md", ReferenceDefCheck{})
		wantSubstrings(t, diags, []string{"unused", `"orphan"`})
	})
	t.Run("collapsed reference", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "See [foo][].\n\n[foo]: https://example.com\n",
		}, "a.md", ReferenceDefCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	t.Run("does not panic on inline code spans", func(t *testing.T) {
		// Regression: inCode was calling .Lines() on *ast.CodeSpan,
		// which panics because CodeSpan is an inline node.
		diags := runCheck(t, map[string]string{
			"a.md": "Use the `flag` package. See `[x][y]` syntax.\n",
		}, "a.md", ReferenceDefCheck{})
		// No reference uses outside code spans, so no diagnostics.
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	t.Run("ignores reference-shaped text inside code span", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "Body has `[text][undefined]` in code, plus real [text][undefined] in prose.\n",
		}, "a.md", ReferenceDefCheck{})
		// Only the prose occurrence should be flagged.
		if len(diags) != 1 {
			t.Fatalf("got %d, want 1: %v", len(diags), diags)
		}
		if !strings.Contains(diags[0].Message, "undefined") {
			t.Errorf("unexpected diagnostic: %v", diags[0])
		}
	})
	t.Run("ignores reference-shaped text inside fenced block", func(t *testing.T) {
		src := "Top.\n\n```\n[text][in-fence]\n```\n\nReal [text][only-real].\n\n[only-real]: x\n"
		diags := runCheck(t, map[string]string{"a.md": src}, "a.md", ReferenceDefCheck{})
		// Only undefined "in-fence" would be flagged if we missed the
		// fence; it is in code so should be skipped.
		for _, d := range diags {
			if strings.Contains(d.Message, "in-fence") {
				t.Errorf("flagged in-fence inside code block: %v", d)
			}
		}
	})
}

func TestCodeFenceLangCheck(t *testing.T) {
	t.Run("no language flagged", func(t *testing.T) {
		src := "Body.\n\n```\nplain\n```\n"
		diags := runCheck(t, map[string]string{"a.md": src}, "a.md", CodeFenceLangCheck{})
		if len(diags) != 1 {
			t.Fatalf("got %d, want 1: %v", len(diags), diags)
		}
	})
	t.Run("language ok", func(t *testing.T) {
		src := "Body.\n\n```go\nfmt.Println(\"hi\")\n```\n"
		diags := runCheck(t, map[string]string{"a.md": src}, "a.md", CodeFenceLangCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
}

func TestHeadingSkipCheck(t *testing.T) {
	t.Run("skip from h1 to h3", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "# Top\n\n### Skip\n",
		}, "a.md", HeadingSkipCheck{})
		wantSubstrings(t, diags, []string{"h1 to h3"})
	})
	t.Run("two h1s", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "# One\n\n# Two\n",
		}, "a.md", HeadingSkipCheck{})
		wantSubstrings(t, diags, []string{"second h1"})
	})
	t.Run("normal sequence ok", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "# Top\n\n## Sub\n\n### Sub-sub\n\n## Another\n",
		}, "a.md", HeadingSkipCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
}

func TestCaseCheck(t *testing.T) {
	// CaseCheck only flags mismatches when both spellings agree
	// case-insensitively. On a case-sensitive filesystem the existing
	// link is what's actually on disk so there's no mismatch to flag;
	// skip the assertion there but exercise the no-op path.
	caseInsensitive := runtime.GOOS == "darwin" || runtime.GOOS == "windows"

	t.Run("matching case ok", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"README.md":     "[d](docs/intro.md)\n",
			"docs/intro.md": "x",
		}, "README.md", CaseCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	if !caseInsensitive {
		t.Skip("case-mismatch detection requires a case-insensitive filesystem")
	}
	t.Run("case mismatch flagged", func(t *testing.T) {
		// Lay the directory tree out by hand so the on-disk casing is
		// guaranteed to be lowercase regardless of map iteration order.
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docs/intro.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		readme := filepath.Join(dir, "README.md")
		if err := os.WriteFile(readme, []byte("[d](Docs/Intro.md)\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		src, err := os.ReadFile(readme)
		if err != nil {
			t.Fatal(err)
		}
		doc := &Document{File: readme, Source: src, Tree: parseTree(src), env: newEnv(Site{})}
		diags, err := CaseCheck{}.Check(doc)
		if err != nil {
			t.Fatal(err)
		}
		wantSubstrings(t, diags, []string{"case mismatch"})
	})
}

func TestRunSelectsMultipleChecks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"),
		[]byte("# Top\n\n[bad](#nope)\n\n[broken](missing.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checks, err := SelectChecks(AllChecks(), []string{"links", "anchors"})
	if err != nil {
		t.Fatal(err)
	}
	diags, err := Run([]string{dir}, checks)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 2 {
		t.Fatalf("got %d, want 2: %v", len(diags), diags)
	}
}

// Component destinations live in attributes, where no link or image
// node ever appears, so the on-disk checks have to read them directly.
func TestComponentDestinations(t *testing.T) {
	t.Run("missing href", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "<Card title=\"G\" href=\"missing.md\">\nbody\n</Card>\n",
		}, "a.md", LinkCheck{})
		wantSubstrings(t, diags, []string{"missing.md"})
	})
	t.Run("present href ok", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "<Card title=\"G\" href=\"b.md\">\nbody\n</Card>\n",
			"b.md": "# B\n",
		}, "a.md", LinkCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
	t.Run("href is not an image", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "<Card title=\"G\" href=\"missing.md\">\nbody\n</Card>\n",
		}, "a.md", ImageCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none: href is LinkCheck's", diags)
		}
	})
	t.Run("assets check reads attributes", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "<Card title=\"G\" href=\"missing.md\">\nbody\n</Card>\n",
		}, "a.md", AssetsCheck{})
		wantSubstrings(t, diags, []string{"missing.md"})
	})
	t.Run("self-closing tag", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "<Card title=\"G\" href=\"missing.md\"/>\n",
		}, "a.md", LinkCheck{})
		wantSubstrings(t, diags, []string{"missing.md"})
	})
	t.Run("external href skipped", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "<Card title=\"G\" href=\"https://example.com\">\nbody\n</Card>\n",
		}, "a.md", LinkCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})
}
