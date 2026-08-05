package mdvet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNavigationCheck(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "covered tree",
			files: map[string]string{
				"SUMMARY.md":    "- [Intro](intro.md)\n- [Guide](docs/guide.md)\n",
				"intro.md":      "# Intro\n",
				"docs/guide.md": "# Guide\n",
			},
		},
		{
			name: "missing target",
			files: map[string]string{
				"SUMMARY.md": "- [Missing](missing.md)\n",
			},
			want: []string{"does not exist"},
		},
		{
			name: "duplicate target",
			files: map[string]string{
				"SUMMARY.md": "- [One](intro.md)\n- [Again](intro.md)\n",
				"intro.md":   "# Intro\n",
			},
			want: []string{"duplicated"},
		},
		{
			name: "omitted file",
			files: map[string]string{
				"SUMMARY.md":   "- [Intro](intro.md)\n",
				"intro.md":     "# Intro\n",
				"extra.md":     "# Extra\n",
				"_site/out.md": "# Generated\n",
				".hidden/a.md": "# Hidden\n",
			},
			want: []string{"omitted from SUMMARY.md"},
		},
		{
			name: "no summary no diagnostics",
			files: map[string]string{
				"intro.md": "# Intro\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, tt.files)
			var target string
			if _, ok := tt.files["SUMMARY.md"]; ok {
				target = "SUMMARY.md"
			} else {
				target = "intro.md"
			}
			diags := runCheckFile(t, dir, target, NavigationCheck{})
			if len(tt.want) == 0 {
				if len(diags) != 0 {
					t.Fatalf("got %v, want none", diags)
				}
				return
			}
			wantSubstrings(t, diags, tt.want)
		})
	}
}

func TestFrontmatterCheck(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "title ok",
			src:  "---\ntitle: Intro\ndraft: false\n---\n# Intro\n",
		},
		{
			name: "missing title",
			src:  "---\ndescription: Intro page\n---\n# Intro\n",
			want: []string{"title is missing"},
		},
		{
			name: "empty title",
			src:  "---\ntitle: \"\"\n---\n# Intro\n",
			want: []string{"title must be a non-empty string"},
		},
		{
			name: "invalid draft",
			src:  "---\ntitle: Intro\ndraft: yes\n---\n# Intro\n",
			want: []string{"draft must be true or false"},
		},
		{
			name: "no frontmatter",
			src:  "# Intro\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := runCheck(t, map[string]string{"a.md": tt.src}, "a.md", FrontmatterCheck{})
			if len(tt.want) == 0 {
				if len(diags) != 0 {
					t.Fatalf("got %v, want none", diags)
				}
				return
			}
			wantSubstrings(t, diags, tt.want)
		})
	}
}

func TestDiagnosticString(t *testing.T) {
	got := (Diagnostic{File: "a.md", Line: 3, Col: 4, Check: "nav", Message: "bad"}).String()
	if got != "a.md:3:4: [nav] bad" {
		t.Fatalf("got %q", got)
	}
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCheckFile(t *testing.T, dir, target string, check Check) []Diagnostic {
	t.Helper()
	file := filepath.Join(dir, target)
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	doc := &Document{File: file, Source: src, Tree: parseTree(src), env: newEnv(Site{})}
	diags, err := check.Check(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diags {
		if d.Check != check.Name() {
			t.Fatalf("diagnostic %v has wrong check name", d)
		}
	}
	return diags
}

func TestSelectChecksAlias(t *testing.T) {
	got, err := SelectChecks(AllChecks(), []string{"nav", "frontmatter"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range got {
		names = append(names, c.Name())
	}
	if strings.Join(names, ",") != "nav,frontmatter" {
		t.Fatalf("got %v", names)
	}
}
