package mdvet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIconCheck(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		file := filepath.Join(root, name)
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return file
	}
	write("docs.json", `{"icons":{"library":"lucide"}}`)
	file := write("page.md", "---\ntitle: Page\nicon: diagram-project\n---\n\n<Card title=\"Known\" icon=\"rocket\"/>\n<Card title=\"Missing\" icon=\"not-a-real-icon\"/>\n")
	diags, err := Run([]string{file}, []Check{IconCheck{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, `"not-a-real-icon"`) {
		t.Fatalf("diagnostics = %#v, want one missing-icon diagnostic", diags)
	}
}

func TestIconCheckFontAwesomeStyle(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "page.md")
	content := "---\ntitle: Page\nicon: rocket\niconType: light\n---\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	diags, err := Run([]string{file}, []Check{IconCheck{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "not available") {
		t.Fatalf("diagnostics = %#v, want unsupported-style diagnostic", diags)
	}
}
