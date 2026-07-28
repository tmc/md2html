package md2html

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderTemplateReturnsLoadError(t *testing.T) {
	dir := t.TempDir()
	const layout = `{{define "layout"}}{{.Missing}}{{end}}`
	if err := os.WriteFile(filepath.Join(dir, "layout.html"), []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := renderTemplate(Config{TemplateDir: dir}, "<p>body</p>", "title", "", false, nil)
	if err == nil {
		t.Fatal("renderTemplate() error = nil, want template execution error")
	}
}
