package mdvet

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// CaseCheck flags relative link or image targets whose path components
// don't match on-disk casing. The check is the value-add on
// case-insensitive filesystems (macOS default, Windows): a link written
// as "Docs/intro.md" can resolve locally yet 404 on the case-sensitive
// production server.
type CaseCheck struct{}

// Name implements [Check].
func (CaseCheck) Name() string { return "case" }

// Check implements [Check].
func (CaseCheck) Check(doc *Document) ([]Diagnostic, error) {
	dir := filepath.Dir(doc.File)
	var diags []Diagnostic

	report := func(n ast.Node, dest, mismatch string) {
		diags = append(diags, Diagnostic{
			File:    doc.File,
			Line:    lineOf(doc.Source, n),
			Check:   "case",
			Message: fmt.Sprintf("link %q: case mismatch — on-disk path is %s", dest, mismatch),
		})
	}

	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest string
		switch node := n.(type) {
		case *ast.Link:
			dest = string(node.Destination)
		case *ast.Image:
			dest = string(node.Destination)
		default:
			return ast.WalkContinue, nil
		}
		if !shouldCheckOnDisk(dest) {
			return ast.WalkContinue, nil
		}
		target, _, err := resolveLink(dir, dest)
		if err != nil {
			return ast.WalkContinue, nil
		}
		// Only check if the target exists case-insensitively but is
		// spelled differently on disk; LinkCheck handles "missing".
		actual, ok := caseResolve(doc.env, target)
		if !ok || actual == target {
			return ast.WalkContinue, nil
		}
		report(n, dest, displayPath(doc.File, actual))
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return diags, nil
}

// caseResolve looks up target against the actual on-disk filenames
// component-by-component, returning the canonical path and true if a
// case-insensitive match exists.
func caseResolve(e *env, target string) (string, bool) {
	target = filepath.Clean(target)
	// Roots ("/" on unix, "C:\\" on windows, ".") are returned as-is.
	if target == "." || isRoot(target) {
		return target, true
	}
	dir, base := filepath.Split(target)
	// Preserve absolute roots: filepath.Split("/var") returns ("/", "var").
	// Trimming the trailing separator turns "/" into "", which would
	// then be treated as relative. Keep it as "/" instead.
	if dir == string(filepath.Separator) {
		// keep dir as-is
	} else {
		dir = strings.TrimRight(dir, string(filepath.Separator))
		if dir == "" {
			dir = "."
		}
	}

	parent, ok := caseResolve(e, dir)
	if !ok {
		return "", false
	}
	if base == "" {
		return parent, true
	}
	entries := e.dirEntries(parent)
	if entries[base] {
		return filepath.Join(parent, base), true
	}
	low := strings.ToLower(base)
	for name := range entries {
		if strings.ToLower(name) == low {
			return filepath.Join(parent, name), true
		}
	}
	return "", false
}

func isRoot(p string) bool {
	if p == string(filepath.Separator) {
		return true
	}
	// Volume roots like "C:\\" on Windows.
	if vol := filepath.VolumeName(p); vol != "" && p == vol+string(filepath.Separator) {
		return true
	}
	return false
}
