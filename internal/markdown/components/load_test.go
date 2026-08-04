package components

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDir writes files into a temporary component directory.
func writeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadRegistry(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"components.json": `{"components":{"Note":{"attrs":["title"],"required":["title"],"template":"note.html"}}}`,
		"note.html":       `<aside><h4>{{.Attrs.title}}</h4>{{.Content}}</aside>`,
	})
	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	note, ok := reg.Lookup("Note")
	if !ok {
		t.Fatal("Note not registered")
	}
	if got := note.Required; len(got) != 1 || got[0] != "title" {
		t.Errorf("Required = %v, want [title]", got)
	}
	// Built-ins remain available alongside loaded components.
	if _, ok := reg.Lookup("Card"); !ok {
		t.Error("built-in Card was dropped")
	}
}

func TestLoadRegistryOverridesBuiltin(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"components.json": `{"components":{"Card":{"attrs":["title"],"template":"card.html"}}}`,
		"card.html":       `<div class="custom">{{.Content}}</div>`,
	})
	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	card, _ := reg.Lookup("Card")
	if len(card.Required) != 0 {
		t.Errorf("override kept built-in Required = %v", card.Required)
	}
}

func TestLoadRegistryErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "missing manifest",
			files: map[string]string{},
			want:  "read components config",
		},
		{
			name:  "malformed manifest",
			files: map[string]string{"components.json": `{`},
			want:  "parse components config",
		},
		{
			name:  "lowercase name",
			files: map[string]string{"components.json": `{"components":{"note":{"template":"n.html"}}}`},
			want:  "uppercase letter",
		},
		{
			name:  "missing template field",
			files: map[string]string{"components.json": `{"components":{"Note":{}}}`},
			want:  "missing template",
		},
		{
			name:  "required not in attrs",
			files: map[string]string{"components.json": `{"components":{"Note":{"required":["title"],"template":"n.html"}}}`},
			want:  `required attribute "title" is not listed in attrs`,
		},
		{
			name:  "missing template file",
			files: map[string]string{"components.json": `{"components":{"Note":{"template":"nope.html"}}}`},
			want:  "no such file",
		},
		{
			name: "unparsable template",
			files: map[string]string{
				"components.json": `{"components":{"Note":{"template":"n.html"}}}`,
				"n.html":          `{{.Content`,
			},
			want: "parse n.html",
		},
		{
			name: "no content reference",
			files: map[string]string{
				"components.json": `{"components":{"Note":{"template":"n.html"}}}`,
				"n.html":          `<aside>nothing</aside>`,
			},
			want: "does not reference {{.Content}}",
		},
		{
			name: "content twice",
			files: map[string]string{
				"components.json": `{"components":{"Note":{"template":"n.html"}}}`,
				"n.html":          `<aside>{{.Content}}{{.Content}}</aside>`,
			},
			want: "more than once",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadRegistry(writeDir(t, tt.files))
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
