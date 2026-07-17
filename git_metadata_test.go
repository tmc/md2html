package md2html

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseGitLastUpdated(t *testing.T) {
	want := map[string]bool{"README.md": true, "missing.md": true}
	out := []byte("1777939200\nREADME.md\n\n1777852800\nother.md\n")
	got := parseGitLastUpdated(out, want)
	if got["README.md"] != "2026-05-05" {
		t.Fatalf("README.md timestamp = %q, want 2026-05-05", got["README.md"])
	}
	if got["missing.md"] != "" {
		t.Fatalf("missing.md timestamp = %q, want empty", got["missing.md"])
	}
}

func TestGitLastUpdatedPaths(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if gitHasHead(dir) {
		t.Fatalf("gitHasHead(empty repo) = true, want false")
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# README\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "add readme")
	if !gitHasHead(dir) {
		t.Fatalf("gitHasHead(committed repo) = false, want true")
	}

	got, err := gitLastUpdatedPaths(dir, []string{"README.md", "missing.md"})
	if err != nil {
		t.Fatalf("gitLastUpdatedPaths() error = %v", err)
	}
	if got["README.md"] == "" {
		t.Fatalf("README.md timestamp is empty")
	}
	if got["missing.md"] != "" {
		t.Fatalf("missing.md timestamp = %q, want empty", got["missing.md"])
	}

	empty := t.TempDir()
	runGit(t, empty, "init")
	got, err = gitLastUpdatedPaths(empty, []string{"missing.md"})
	if err != nil {
		t.Fatalf("gitLastUpdatedPaths(empty) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty repo timestamps = %#v, want empty", got)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
