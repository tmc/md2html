package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "T")
	write("docs/index.md", "# Home\n\nSee [missing](missing.md).\n")
	write("docs/agents/agents.md", "# Agents\n\nAgents call tools in a loop until the task is done.\n")
	write("docs/gone.md", "# Gone\n")
	write("docs/replaced.md", "# Replaced\n")
	git("add", ".")
	git("commit", "-q", "-m", "one")
	git("tag", "v1.0.0")
	git("mv", "docs/agents/agents.md", "docs/agents.md")
	git("rm", "-q", "docs/gone.md", "docs/replaced.md")
	write("docs/index.md", "# Home\n")
	write("docs/new.md", "---\ntitle: New\naliases: [replaced, ghost]\n---\n\nNew.\n")
	git("add", ".")
	git("commit", "-q", "-m", "two")

	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	code := run([]string{"-versions", "-version-pattern", "v*", "-c", "links", filepath.Join(dir, "docs")}, stdout, stderr)
	out, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	errOut, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("exit code %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	got := string(out)
	for _, want := range []string{
		"v1.0.0:docs/index.md:3:",               // the tag's own broken link
		`[version-aliases] alias "ghost" names`, // an alias to nothing
		"v1.0.0:docs/gone.md:1:1: [version-removed]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"agents/agents.md:1:1: [version-removed]", "replaced.md:1:1: [version-removed]", `alias "replaced"`} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output contains %q:\n%s", unwanted, got)
		}
	}
	if code := run([]string{"-versions", dir, dir}, stdout, stderr); code != 2 {
		t.Errorf("-versions with two paths: exit code %d, want 2", code)
	}
}
