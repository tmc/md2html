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

func TestRunAssetsFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("![demo](demo.mp4)\n"), 0o644); err != nil {
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
	code := run([]string{"-checks=assets", dir}, stdout, stderr)
	if code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	out, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "[assets] media \"demo.mp4\"") {
		t.Fatalf("stdout missing asset diagnostic:\n%s", got)
	}
}

func TestRunIncludeRootFlag(t *testing.T) {
	repo := t.TempDir()
	for name, content := range map[string]string{
		"examples/main.go": "package main\n",
		"docs/page.md":     "# Page\n\n<Code src=\"/examples/main.go\" />\n",
	} {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	docs := filepath.Join(repo, "docs")
	tests := []struct {
		args []string
		code int
	}{
		{[]string{"-checks=includes", docs}, 1},
		{[]string{"-checks=includes", "-include-root", repo, docs}, 0},
	}
	for _, tt := range tests {
		stdout, err := os.CreateTemp(t.TempDir(), "stdout")
		if err != nil {
			t.Fatal(err)
		}
		if code := run(tt.args, stdout, os.Stderr); code != tt.code {
			out, _ := os.ReadFile(stdout.Name())
			t.Errorf("mdvet %s: exit code %d, want %d\n%s", strings.Join(tt.args, " "), code, tt.code, out)
		}
	}
}
