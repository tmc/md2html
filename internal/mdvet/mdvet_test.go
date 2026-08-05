package mdvet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkCheck(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string // relative path -> contents
		target   string            // file to vet
		wantBad  []string          // substrings that must each appear in some diagnostic
		wantNone bool              // expect zero diagnostics
	}{
		{
			name: "valid relative link",
			files: map[string]string{
				"README.md":     "See [docs](docs/intro.md).\n",
				"docs/intro.md": "# Intro\n",
			},
			target:   "README.md",
			wantNone: true,
		},
		{
			name: "missing file",
			files: map[string]string{
				"README.md": "See [docs](docs/missing.md).\n",
			},
			target:  "README.md",
			wantBad: []string{`docs/missing.md`},
		},
		{
			name: "directory target ok",
			files: map[string]string{
				"README.md":  "See [examples](examples).\n",
				"examples/a": "x",
			},
			target:   "README.md",
			wantNone: true,
		},
		{
			name: "anchor on existing file ok",
			files: map[string]string{
				"README.md":     "See [intro](docs/intro.md#start).\n",
				"docs/intro.md": "# Start\n",
			},
			target:   "README.md",
			wantNone: true,
		},
		{
			name: "external link skipped",
			files: map[string]string{
				"README.md": "See [google](https://example.com).\n",
			},
			target:   "README.md",
			wantNone: true,
		},
		{
			name: "fragment-only link skipped",
			files: map[string]string{
				"README.md": "See [top](#top).\n",
			},
			target:   "README.md",
			wantNone: true,
		},
		{
			name: "mailto link skipped",
			files: map[string]string{
				"README.md": "Mail [me](mailto:me@example.com).\n",
			},
			target:   "README.md",
			wantNone: true,
		},
		{
			name: "anchor on directory flagged",
			files: map[string]string{
				"README.md":  "See [examples](examples#here).\n",
				"examples/a": "x",
			},
			target:  "README.md",
			wantBad: []string{"anchor on directory"},
		},
		{
			name: "percent-encoded path",
			files: map[string]string{
				"README.md": "See [doc](my%20doc.md).\n",
				"my doc.md": "# my doc\n",
			},
			target:   "README.md",
			wantNone: true,
		},
		{
			name: "absolute path flagged",
			files: map[string]string{
				"README.md": "See [leaked](/Users/me/notes.md).\n",
			},
			target:  "README.md",
			wantBad: []string{"absolute path"},
		},
		{
			name: "absolute path flagged regardless of existence",
			files: map[string]string{
				// Use a path that is overwhelmingly likely to exist on
				// the host (/etc) — diagnostic must still fire.
				"README.md": "See [etc](/etc).\n",
			},
			target:  "README.md",
			wantBad: []string{"absolute path"},
		},
		{
			name: "parent-relative link",
			files: map[string]string{
				"docs/guide.md": "See [readme](../README.md).\n",
				"README.md":     "# top\n",
			},
			target:   "docs/guide.md",
			wantNone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, content := range tt.files {
				p := filepath.Join(dir, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			file := filepath.Join(dir, tt.target)
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			doc := &Document{File: file, Source: src, Tree: parseTree(src), env: newEnv(Site{})}
			diags, err := LinkCheck{}.Check(doc)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantNone {
				if len(diags) != 0 {
					t.Fatalf("got %d diagnostics, want 0: %v", len(diags), diags)
				}
				return
			}
			for _, want := range tt.wantBad {
				found := false
				for _, d := range diags {
					if strings.Contains(d.Message, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no diagnostic mentions %q; got %v", want, diags)
				}
			}
		})
	}
}

func TestRunWalksDirectory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.md":          "[ok](b.md)\n",
		"b.md":          "[bad](missing.md)\n",
		"sub/c.md":      "[up](../a.md)\n",
		"not-a-doc.txt": "ignored",
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	diags, err := Run([]string{dir}, AllChecks())
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].File, "b.md") {
		t.Errorf("diagnostic on wrong file: %v", diags[0])
	}
	if !strings.Contains(diags[0].Message, "missing.md") {
		t.Errorf("diagnostic does not mention missing.md: %v", diags[0])
	}
}

func TestSelectChecks(t *testing.T) {
	all := AllChecks()
	got, err := SelectChecks(all, []string{"links"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name() != "links" {
		t.Fatalf("unexpected selection: %v", got)
	}
	if _, err := SelectChecks(all, []string{"nope"}); err == nil {
		t.Fatal("expected error for unknown check")
	}
}
