package mdvet

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// NavigationCheck verifies that SUMMARY.md covers the markdown tree.
type NavigationCheck struct{}

// Name implements [Check].
func (NavigationCheck) Name() string { return "nav" }

var summaryLinkRE = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

// Check implements [Check].
func (NavigationCheck) Check(doc *Document) ([]Diagnostic, error) {
	if filepath.Base(doc.File) != "SUMMARY.md" {
		return nil, nil
	}
	root := filepath.Dir(doc.File)
	refs := make(map[string]int)
	var diags []Diagnostic

	for i, line := range strings.Split(string(doc.Source), "\n") {
		for _, m := range summaryLinkRE.FindAllStringSubmatchIndex(line, -1) {
			dest := line[m[2]:m[3]]
			if !summaryTarget(dest) {
				continue
			}
			target, _, err := resolveLink(root, dest)
			if err != nil {
				diags = append(diags, navDiag(doc.File, i+1, m[2]+1, fmt.Sprintf("invalid target %q: %v", dest, err)))
				continue
			}
			if !isMarkdown(target) {
				continue
			}
			rel, err := filepath.Rel(root, target)
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(filepath.Clean(rel))
			if prev, ok := refs[rel]; ok {
				diags = append(diags, navDiag(doc.File, i+1, m[2]+1, fmt.Sprintf("%s duplicated; first listed at line %d", rel, prev)))
			}
			refs[rel] = i + 1
			if _, err := os.Stat(target); err != nil {
				diags = append(diags, navDiag(doc.File, i+1, m[2]+1, fmt.Sprintf("%s does not exist", rel)))
			}
		}
	}

	files, err := summaryMarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	for _, rel := range files {
		if _, ok := refs[rel]; !ok {
			diags = append(diags, Diagnostic{
				File:    filepath.Join(root, filepath.FromSlash(rel)),
				Line:    1,
				Col:     1,
				Check:   "nav",
				Message: "markdown file omitted from SUMMARY.md",
			})
		}
	}
	return diags, nil
}

func navDiag(file string, line, col int, msg string) Diagnostic {
	return Diagnostic{File: file, Line: line, Col: col, Check: "nav", Message: msg}
}

func summaryTarget(dest string) bool {
	if dest == "" || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "//") {
		return false
	}
	u, err := url.Parse(dest)
	return err == nil && u.Scheme == ""
}

func summaryMarkdownFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || name == "output" || name == "_site") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || filepath.Base(path) == "SUMMARY.md" || !isMarkdown(path) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}
