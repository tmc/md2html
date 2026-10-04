package md2html

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindMarkdownFilesSkipsUnreadableDir verifies that an unreadable
// subdirectory (e.g. a permission-denied system directory like .Trashes) is
// skipped rather than aborting the entire listing.
func TestFindMarkdownFilesSkipsUnreadableDir(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "readme.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	denied := filepath.Join(root, "denied")
	if err := os.Mkdir(denied, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(denied, "hidden.md"), []byte("# nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(denied, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(denied, 0o755) })

	// Skip if the test runner can read the directory anyway (e.g. running as
	// root), since the permission denial wouldn't occur.
	if _, err := os.ReadDir(denied); err == nil {
		t.Skip("unreadable directory is readable in this environment")
	}

	files, err := findMarkdownFiles(root, 5)
	if err != nil {
		t.Fatalf("findMarkdownFiles returned error for unreadable subdir: %v", err)
	}

	var got []string
	for _, f := range files {
		got = append(got, f.RelPath)
	}
	if len(got) != 1 || got[0] != "readme.md" {
		t.Errorf("expected only readme.md, got %v", got)
	}
}

func TestFindMarkdownFilesDepth(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "guide", "advanced"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.md", "guide/start.md", "guide/advanced/deep.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("# x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name  string
		depth int
		want  int
	}{
		{"negative means default", -1, 2},
		{"zero means default", 0, 2},
		{"root only", 1, 1},
		{"one level down", 2, 2},
		{"two levels down", 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := findMarkdownFiles(root, tt.depth)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != tt.want {
				t.Errorf("findMarkdownFiles(root, %d) found %d files, want %d", tt.depth, len(files), tt.want)
			}
		})
	}
}
