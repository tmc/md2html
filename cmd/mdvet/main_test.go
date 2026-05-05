package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunChecksFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\ndraft: maybe\n---\n# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	code := run([]string{"-checks=frontmatter", dir}, stdout, stderr)
	if code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	out, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, ":1:1: [frontmatter]") {
		t.Fatalf("stdout missing diagnostic prefix:\n%s", got)
	}
	if strings.Contains(got, "[nav]") {
		t.Fatalf("-checks=frontmatter ran nav:\n%s", got)
	}
}
