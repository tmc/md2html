package mdvet

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

// gitTree writes files into a fresh temporary repository and returns
// its directory. Tests that need a tracked file run "git add" on it;
// check-ignore consults the index, so no commit is required.
func gitTree(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
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
	git(t, dir, "init")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// checkGitIgnored runs GitIgnoredCheck on target within dir.
func checkGitIgnored(t *testing.T, dir, target string, site Site) []Diagnostic {
	t.Helper()
	file := filepath.Join(dir, target)
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if site.Root == "" && site.Base != "" {
		site.Root = dir
	}
	doc := &Document{File: file, Source: src, Tree: parseTree(src), env: newEnv(site)}
	diags, err := GitIgnoredCheck{}.Check(doc)
	if err != nil {
		t.Fatal(err)
	}
	return diags
}

func TestGitIgnoredCheck(t *testing.T) {
	t.Run("ignored target flagged", func(t *testing.T) {
		dir := gitTree(t, map[string]string{
			".gitignore":      "build/\n",
			"a.md":            "[report](build/report.md) and ![pic](build/pic.png)\n",
			"build/report.md": "# Report\n",
			"build/pic.png":   "x",
		})
		diags := checkGitIgnored(t, dir, "a.md", Site{})
		wantSubstrings(t, diags, []string{"build/report.md", "build/pic.png"})
		if len(diags) != 2 {
			t.Errorf("got %d diagnostics, want 2: %v", len(diags), diags)
		}
	})

	t.Run("tracked target not flagged", func(t *testing.T) {
		// A file git already tracks ships with the repository even
		// though a pattern matches it.
		dir := gitTree(t, map[string]string{
			".gitignore": "*.md\n",
			"a.md":       "[other](b.md)\n",
			"b.md":       "# B\n",
		})
		git(t, dir, "add", "-f", "b.md")
		if diags := checkGitIgnored(t, dir, "a.md", Site{}); len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})

	t.Run("unignored target silent", func(t *testing.T) {
		dir := gitTree(t, map[string]string{
			".gitignore": "build/\n",
			"a.md":       "[other](b.md)\n",
			"b.md":       "# B\n",
		})
		if diags := checkGitIgnored(t, dir, "a.md", Site{}); len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})

	t.Run("missing target silent", func(t *testing.T) {
		// A link to nothing is LinkCheck's finding, not this one's.
		dir := gitTree(t, map[string]string{
			".gitignore": "build/\n",
			"a.md":       "[gone](build/missing.md)\n",
		})
		if diags := checkGitIgnored(t, dir, "a.md", Site{}); len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})

	t.Run("outside a repository silent", func(t *testing.T) {
		diags := runCheck(t, map[string]string{
			"a.md": "[other](b.md)\n",
			"b.md": "# B\n",
		}, "a.md", GitIgnoredCheck{})
		if len(diags) != 0 {
			t.Errorf("got %v, want none", diags)
		}
	})

	t.Run("site URL resolving to ignored source", func(t *testing.T) {
		dir := gitTree(t, map[string]string{
			".gitignore":    "drafts/\n",
			"a.md":          "[draft](/docs/drafts/new)\n",
			"drafts/new.md": "# New\n",
		})
		diags := checkGitIgnored(t, dir, "a.md", Site{Base: "/docs"})
		wantSubstrings(t, diags, []string{"ignored by git"})
	})

	t.Run("absolute path silent", func(t *testing.T) {
		dir := gitTree(t, map[string]string{
			".gitignore": "build/\n",
			"a.md":       "[x](/Users/me/notes.md)\n",
		})
		if diags := checkGitIgnored(t, dir, "a.md", Site{}); len(diags) != 0 {
			t.Errorf("got %v, want none — LinkCheck reports absolute paths", diags)
		}
	})
}
