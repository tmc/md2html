package ignorefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetMatch(t *testing.T) {
	// The patterns are those a real ignore file uses: a directory, a
	// glob, bare directory names, and one exact path.
	set := &Set{root: "/repo"}
	for _, line := range []string{
		"# a comment",
		"",
		"docs/planning/",
		"bakeoff-*.md",
		"skills/",
		"testdata/",
		"cmd/chdb/README.md",
	} {
		if p, ok := parsePattern(line); ok {
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

func TestSetNegation(t *testing.T) {
	set := &Set{root: "/repo"}
	for _, line := range []string{"drafts/", "!drafts/keep.md"} {
		if p, ok := parsePattern(line); ok {
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

// TestSetNilExcludesNothing covers the common case: most trees
// have no ignore file at all.
func TestSetNilExcludesNothing(t *testing.T) {
	var set *Set
	if set.Excludes("/anywhere/at/all.md", false) {
		t.Error("a nil Set excluded something")
	}
	if set.match("anything", false) {
		t.Error("a nil Set matched something")
	}
}

// TestFind covers which file is chosen when there is more than
// one: the preferred name within a directory, but the nearest directory
// before any name preference.
func TestFind(t *testing.T) {
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

	got, ok := find(filepath.Join(repo, "docs"))
	if !ok || got != filepath.Join(repo, ".mintignore") {
		t.Errorf("find = %q, %v; want the .mintignore above the tree", got, ok)
	}

	// Same directory, both names: the native one wins.
	touch(".md2htmlignore")
	got, ok = find(filepath.Join(repo, "docs"))
	if !ok || got != filepath.Join(repo, ".md2htmlignore") {
		t.Errorf("find = %q, %v; want .md2htmlignore to be preferred", got, ok)
	}

	// A nearer file wins over a preferred name further away.
	touch("docs/.mintignore")
	got, ok = find(filepath.Join(repo, "docs"))
	if !ok || got != filepath.Join(repo, "docs", ".mintignore") {
		t.Errorf("find = %q, %v; want the nearer file", got, ok)
	}
}
