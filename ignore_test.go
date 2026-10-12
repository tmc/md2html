package md2html

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindMarkdownFilesHonoursIgnoreFile is the point of the change: an
// ignore file usually sits at the repository root while the tree being
// served is a subdirectory, so the patterns are relative to a directory
// above the one walked.
func TestFindMarkdownFilesHonoursIgnoreFile(t *testing.T) {
	repo := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".mintignore", "# notes\ndocs/planning/\nbakeoff-*.md\n")
	write("docs/index.md", "# Home\n")
	write("docs/guide.md", "# Guide\n")
	write("docs/planning/roadmap.md", "# Secret\n")
	write("docs/bakeoff-2026.md", "# Scratch\n")

	files, err := findMarkdownFiles(filepath.Join(repo, "docs"), 10)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(files))
	for _, f := range files {
		got[filepath.ToSlash(f.RelPath)] = true
	}

	for _, want := range []string{"index.md", "guide.md"} {
		if !got[want] {
			t.Errorf("%s missing from %v", want, got)
		}
	}
	for _, excluded := range []string{"planning/roadmap.md", "bakeoff-2026.md"} {
		if got[excluded] {
			t.Errorf("%s was published despite the ignore file; got %v", excluded, got)
		}
	}
}

func TestFindMarkdownFilesWithoutIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := findMarkdownFiles(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("got %v, want the one file: no ignore file means exclude nothing", files)
	}
}
