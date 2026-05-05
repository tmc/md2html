package mdvet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/md2html/internal/media"
	"github.com/yuin/goldmark/ast"
)

// AssetsCheck verifies local link, image, and media references.
type AssetsCheck struct{}

// Name implements [Check].
func (AssetsCheck) Name() string { return "assets" }

// Check implements [Check].
func (AssetsCheck) Check(doc *Document) ([]Diagnostic, error) {
	dir := filepath.Dir(doc.File)
	localIDs := make(map[string]bool)
	collectHeadingIDs(doc.Tree, localIDs)
	var diags []Diagnostic

	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		dest, ok := nodeDest(n)
		if !ok {
			return ast.WalkContinue, nil
		}
		line := lineOf(doc.Source, n)
		if frag, ok := strings.CutPrefix(dest, "#"); ok {
			if frag != "" && !localIDs[frag] {
				diags = append(diags, assetDiag(doc.File, line, fmt.Sprintf("link %q: no heading with id %q in this file", dest, frag)))
			}
			return ast.WalkContinue, nil
		}
		if !shouldCheckOnDisk(dest) {
			return ast.WalkContinue, nil
		}
		if isAbsolutePathLink(dest) {
			diags = append(diags, assetDiag(doc.File, line, fmt.Sprintf("link %q: absolute path; mdvet refuses to validate", dest)))
			return ast.WalkContinue, nil
		}
		target, frag, err := resolveLink(dir, dest)
		if err != nil {
			diags = append(diags, assetDiag(doc.File, line, fmt.Sprintf("invalid link %q: %v", dest, err)))
			return ast.WalkContinue, nil
		}
		info, err := os.Stat(target)
		if err != nil {
			kind := "link"
			if media.IsMediaPath(dest) {
				kind = "media"
			}
			diags = append(diags, assetDiag(doc.File, line, fmt.Sprintf("%s %q: %s does not exist", kind, dest, displayPath(doc.File, target))))
			return ast.WalkContinue, nil
		}
		if info.IsDir() {
			if frag != "" {
				diags = append(diags, assetDiag(doc.File, line, fmt.Sprintf("link %q: anchor on directory target %s", dest, displayPath(doc.File, target))))
			}
			return ast.WalkContinue, nil
		}
		if frag != "" && isMarkdown(target) {
			ids := doc.env.anchorsFor(target)
			if len(ids) != 0 && !ids[frag] {
				diags = append(diags, assetDiag(doc.File, line, fmt.Sprintf("link %q: %s has no heading with id %q", dest, displayPath(doc.File, target), frag)))
			}
		}
		return ast.WalkContinue, nil
	})
	return diags, err
}

func nodeDest(n ast.Node) (string, bool) {
	switch n := n.(type) {
	case *ast.Link:
		return string(n.Destination), true
	case *ast.Image:
		return string(n.Destination), true
	}
	return "", false
}

func assetDiag(file string, line int, msg string) Diagnostic {
	return Diagnostic{File: file, Line: line, Col: 1, Check: "assets", Message: msg}
}
