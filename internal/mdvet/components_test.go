package mdvet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/md2html/internal/markdown/components"
)

func TestComponentCheck(t *testing.T) {
	dir := t.TempDir()
	src := "# Doc\n\n<Widget/>\n\n<Card/>\n\n<Card title=\"a\" nope=\"b\"/>\n"
	file := filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	diags, err := Run([]string{file}, []Check{ComponentCheck{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(diags) != 3 {
		t.Fatalf("got %d diagnostics, want 3: %v", len(diags), diags)
	}
	want := []struct {
		line   int
		substr string
	}{
		{3, "unknown component <Widget> (no -components directory configured; known: Card, CardGroup)"},
		{5, `missing required attribute "title"`},
		{7, `no attribute "nope"`},
	}
	for i, w := range want {
		if diags[i].Line != w.line {
			t.Errorf("diag %d line = %d, want %d", i, diags[i].Line, w.line)
		}
		if !strings.Contains(diags[i].Message, w.substr) {
			t.Errorf("diag %d message = %q, want it to contain %q", i, diags[i].Message, w.substr)
		}
		if diags[i].Check != "components" {
			t.Errorf("diag %d check = %q, want components", i, diags[i].Check)
		}
	}
}

// TestComponentCheckConfigured verifies that a supplied registry both
// silences its own components and drops the hint about the missing
// directory.
func TestComponentCheckConfigured(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "components.json"),
		[]byte(`{"components":{"Note":{"attrs":["title"],"template":"n.html"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "n.html"), []byte(`<aside>{{.Content}}</aside>`), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := components.LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	docs := t.TempDir()
	file := filepath.Join(docs, "a.md")
	if err := os.WriteFile(file, []byte("<Note>\nx\n</Note>\n\n<Widget/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diags, err := Run([]string{file}, []Check{ComponentCheck{Registry: reg, Configured: true}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %v", len(diags), diags)
	}
	if strings.Contains(diags[0].Message, "no -components directory configured") {
		t.Errorf("configured registry still reported the missing directory: %q", diags[0].Message)
	}
	if !strings.Contains(diags[0].Message, "known: Card, CardGroup, Note") {
		t.Errorf("message %q does not list the loaded components", diags[0].Message)
	}
}

// TestComponentCheckCleanDocument guards against the check firing on
// documents that use no components at all, which is why it can be on by
// default.
func TestComponentCheckCleanDocument(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	body := "# Doc\n\nText with <em>inline html</em> and a <div>\nblock\n</div>\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	diags, err := Run([]string{file}, []Check{ComponentCheck{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("want no diagnostics, got %v", diags)
	}
}
