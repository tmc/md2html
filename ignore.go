package md2html

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ignoreNames are the file names an exclusion list is read from, in
// order of preference. ".mintignore" is read because repositories that
// already publish with another renderer declare their exclusions there,
// and md2html should not publish what they have excluded.
var ignoreNames = []string{".md2htmlignore", ".mintignore"}

// ignoreSet holds the exclusions declared by an ignore file, which names
// the paths a repository keeps out of its published site: drafts,
// planning notes, reference trees that belong somewhere else.
//
// Honoring it matters more than tidiness. Without it "md2html -html"
// publishes whatever the repository deliberately excludes, and the only
// sign is a page count that nobody counted.
//
// The syntax is gitignore's, of which this implements the part such
// files actually use: comments, "!" negation, a trailing "/" for
// directories only, a leading "/" or an interior "/" to anchor a
// pattern to the file's own directory, and "*" globbing within a path
// segment. Patterns naming a single segment match at any depth.
type ignoreSet struct {
	// root is the directory the patterns are relative to: the one
	// holding the ignore file, which is often the repository rather
	// than the directory being served.
	root     string
	patterns []ignorePattern
}

type ignorePattern struct {
	segs     []string
	dirOnly  bool
	negate   bool
	anchored bool
}

// findIgnoreFile returns the ignore file governing dir: the first of
// [ignoreNames] present at or above it. A docs tree is usually a
// subdirectory of the repository that excludes things, so the search
// walks up. A directory is settled before the next one up is tried, so
// a nearer file wins over a preferred name further away.
func findIgnoreFile(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		for _, name := range ignoreNames {
			file := filepath.Join(dir, name)
			if _, err := os.Stat(file); err == nil {
				return file, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// loadIgnoreSet reads the ignore file governing dir. It returns nil when
// there is none, which callers treat as "exclude nothing".
func loadIgnoreSet(dir string) (*ignoreSet, error) {
	file, ok := findIgnoreFile(dir)
	if !ok {
		return nil, nil
	}
	root := filepath.Dir(file)
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	set := &ignoreSet{root: root}
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		if p, ok := parseIgnorePattern(scan.Text()); ok {
			set.patterns = append(set.patterns, p)
		}
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if len(set.patterns) == 0 {
		return nil, nil
	}
	return set, nil
}

func parseIgnorePattern(line string) (ignorePattern, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ignorePattern{}, false
	}
	var p ignorePattern
	if rest, ok := strings.CutPrefix(line, "!"); ok {
		p.negate = true
		line = rest
	}
	if rest, ok := strings.CutSuffix(line, "/"); ok {
		p.dirOnly = true
		line = rest
	}
	// A leading slash anchors, and so does any interior one: "docs/planning"
	// names that one directory, while "planning" names any of them.
	if rest, ok := strings.CutPrefix(line, "/"); ok {
		p.anchored = true
		line = rest
	} else if strings.Contains(line, "/") {
		p.anchored = true
	}
	if line == "" {
		return ignorePattern{}, false
	}
	p.segs = strings.Split(line, "/")
	return p, true
}

// match reports whether the path rel, given in slash form relative to
// the set's root, is excluded. A later pattern wins, so a "!" line can
// bring back something an earlier line excluded.
func (s *ignoreSet) match(rel string, isDir bool) bool {
	if s == nil {
		return false
	}
	segs := strings.Split(path.Clean(rel), "/")
	excluded := false
	for _, p := range s.patterns {
		if p.matches(segs, isDir) {
			excluded = !p.negate
		}
	}
	return excluded
}

// excludes reports whether the file at absolute path abs is excluded.
func (s *ignoreSet) excludes(abs string, isDir bool) bool {
	if s == nil {
		return false
	}
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		// Outside the tree the patterns describe.
		return false
	}
	return s.match(filepath.ToSlash(rel), isDir)
}

func (p ignorePattern) matches(segs []string, isDir bool) bool {
	if p.anchored {
		return p.matchesAt(segs, 0, isDir)
	}
	for i := range segs {
		if p.matchesAt(segs, i, isDir) {
			return true
		}
	}
	return false
}

func (p ignorePattern) matchesAt(segs []string, start int, isDir bool) bool {
	if start+len(p.segs) > len(segs) {
		return false
	}
	for i, want := range p.segs {
		if want == "**" {
			// Anything at this position; the remaining segments are
			// checked from every later offset.
			for j := start + i; j <= len(segs)-(len(p.segs)-i-1); j++ {
				if (ignorePattern{segs: p.segs[i+1:], dirOnly: p.dirOnly, anchored: true}).matchesAt(segs, j, isDir) {
					return true
				}
			}
			return false
		}
		ok, err := path.Match(want, segs[start+i])
		if err != nil || !ok {
			return false
		}
	}
	// A directory-only pattern matches a directory: either the match
	// stopped short of the final segment, or the final segment is one.
	if p.dirOnly {
		return start+len(p.segs) < len(segs) || isDir
	}
	return true
}
