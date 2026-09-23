package md2html

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoreSetMatch(t *testing.T) {
	// The patterns are those a real ignore file uses: a directory, a
	// glob, bare directory names, and one exact path.
	set := &ignoreSet{root: "/repo"}
	for _, line := range []string{
		"# a comment",
		"",
		"docs/planning/",
		"bakeoff-*.md",
		"skills/",
		"testdata/",
		"cmd/chdb/README.md",
	} {
		if p, ok := parseIgnorePattern(line); ok {
			set.patterns = append(set.patterns, p)
		}
	}

	tests := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{"docs/planning/notes.md", false, true},
		{"docs/planning", true, true},
		{"docs/quickstart.md", false, false},
		// Anchored: only the docs/planning directory, not any planning.
		{"other/planning/notes.md", false, false},

		{"bakeoff-2026.md", false, true},
		{"docs/deep/bakeoff-x.md", false, true},
		{"bakeoff.md", false, false},

		// Unanchored directory names match at any depth.
		{"skills/intro.md", false, true},
		{"docs/skills/intro.md", false, true},
		{"cdpscripttest/testdata/x.md", false, true},
		// ...but only as a directory, never as a file of that name.
		{"docs/skills", false, false},

		{"cmd/chdb/README.md", false, true},
		{"cmd/other/README.md", false, false},
	}
	for _, tt := range tests {
		if got := set.match(tt.rel, tt.isDir); got != tt.want {
			t.Errorf("match(%q, dir=%v) = %v, want %v", tt.rel, tt.isDir, got, tt.want)
		}
	}
}

func TestIgnoreSetNegation(t *testing.T) {
	set := &ignoreSet{root: "/repo"}
	for _, line := range []string{"drafts/", "!drafts/keep.md"} {
		if p, ok := parseIgnorePattern(line); ok {
			set.patterns = append(set.patterns, p)
		}
	}
	if !set.match("drafts/wip.md", false) {
		t.Error("drafts/wip.md should be excluded")
	}
	if set.match("drafts/keep.md", false) {
		t.Error("drafts/keep.md was brought back by ! and should not be excluded")
	}
}

// TestIgnoreSetNilExcludesNothing covers the common case: most trees
// have no ignore file at all.
func TestIgnoreSetNilExcludesNothing(t *testing.T) {
	var set *ignoreSet
	if set.excludes("/anywhere/at/all.md", false) {
		t.Error("a nil ignoreSet excluded something")
	}
	if set.match("anything", false) {
		t.Error("a nil ignoreSet matched something")
	}
}

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

// TestFindIgnoreFile covers which file is chosen when there is more than
// one: the preferred name within a directory, but the nearest directory
// before any name preference.
func TestFindIgnoreFile(t *testing.T) {
	repo := t.TempDir()
	touch := func(rel string) {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	touch("docs/index.md")
	touch(".mintignore")

	got, ok := findIgnoreFile(filepath.Join(repo, "docs"))
	if !ok || got != filepath.Join(repo, ".mintignore") {
		t.Errorf("findIgnoreFile = %q, %v; want the .mintignore above the tree", got, ok)
	}

	// Same directory, both names: the native one wins.
	touch(".md2htmlignore")
	got, ok = findIgnoreFile(filepath.Join(repo, "docs"))
	if !ok || got != filepath.Join(repo, ".md2htmlignore") {
		t.Errorf("findIgnoreFile = %q, %v; want .md2htmlignore to be preferred", got, ok)
	}

	// A nearer file wins over a preferred name further away.
	touch("docs/.mintignore")
	got, ok = findIgnoreFile(filepath.Join(repo, "docs"))
	if !ok || got != filepath.Join(repo, "docs", ".mintignore") {
		t.Errorf("findIgnoreFile = %q, %v; want the nearer file", got, ok)
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
