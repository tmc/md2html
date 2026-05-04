package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderTemplateThreadsVersionData verifies that Version and Versions
// from RenderOptions reach the template payload. The shared renderer used
// to hardcode them to zero values, which silently dropped real data the
// server resolved into s.versions.
func TestRenderTemplateThreadsVersionData(t *testing.T) {
	dir := t.TempDir()
	tmpl := `{{define "layout"}}` +
		`<v>{{.Version}}</v>` +
		`{{range .Versions}}<x name="{{.Name}}" ref="{{.Ref}}" tag="{{.IsTag}}"/>{{end}}` +
		`{{end}}`
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{TemplateDir: dir}
	opts := RenderOptions{
		Version: "v1.2.3",
		Versions: []GitVersion{
			{Name: "v1.2.3", Ref: "refs/tags/v1.2.3", IsTag: true, Commit: "abc1234"},
			{Name: "main", Ref: "refs/heads/main", IsTag: false, Commit: "def5678"},
		},
	}

	got := renderTemplateWithOptions(cfg, "<p>body</p>", "title", "", false, map[string]interface{}{}, opts)

	want := `<v>v1.2.3</v><x name="v1.2.3" ref="refs/tags/v1.2.3" tag="true"/><x name="main" ref="refs/heads/main" tag="false"/>`
	if !strings.Contains(got, want) {
		t.Fatalf("rendered output missing version data\nwant substring: %s\ngot: %s", want, got)
	}
}

// TestRenderTemplateEmptyVersionsByDefault verifies that when no versioning
// is configured, Version is empty and Versions has zero length — confirming
// we are not silently injecting noise.
func TestRenderTemplateEmptyVersionsByDefault(t *testing.T) {
	dir := t.TempDir()
	tmpl := `{{define "layout"}}<v>{{.Version}}</v><n>{{len .Versions}}</n>{{end}}`
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{TemplateDir: dir}
	got := renderTemplateWithOptions(cfg, "", "", "", false, map[string]interface{}{}, RenderOptions{})
	if !strings.Contains(got, `<v></v><n>0</n>`) {
		t.Fatalf("expected empty version payload, got: %s", got)
	}
}
