package mdvet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzMdvet(f *testing.F) {
	for _, seed := range []string{
		"",
		"# Title\n\n[missing](missing.md#x)\n",
		"---\ntitle: Test\ndraft: false\n---\n# Test\n",
		"# A\n### C\n\n```\ncode\n```\n",
		"[x][ref]\n\n[ref]: target.md\n",
		"![img](pic.png)\n",
		"> [!NOTE]\n> note\n",
		"nul\x00byte\n\n[bad](%zz)\n",
		strings.Repeat("# h\n", 128),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, markdown string) {
		if len(markdown) > 32<<10 {
			t.Skip()
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic: %v\ninput=%q", r, markdown)
			}
		}()
		dir := t.TempDir()
		writeFuzzFile(t, filepath.Join(dir, "page.md"), []byte(markdown))
		writeFuzzFile(t, filepath.Join(dir, "target.md"), []byte("# Target\n"))
		if _, err := Run([]string{dir}, AllChecks()); err != nil {
			t.Fatalf("run mdvet: %v", err)
		}
	})
}

func writeFuzzFile(t *testing.T, name string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
