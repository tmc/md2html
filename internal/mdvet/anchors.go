package mdvet

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tmc/md2html/internal/anchor"
	"github.com/yuin/goldmark/ast"
)

// AnchorCheck verifies that "#section" fragments in links resolve to a
// real heading in the target document. Anchors on local fragment-only
// links are checked against the current document's headings; anchors on
// links to other Markdown files are checked against that file's
// headings. Anchors targeting non-Markdown files are not checked.
type AnchorCheck struct{}

// Name implements [Check].
func (AnchorCheck) Name() string { return "anchors" }

// Check implements [Check].
func (AnchorCheck) Check(doc *Document) ([]Diagnostic, error) {
	localIDs := make(map[string]bool)
	collectHeadingIDs(doc.Tree, localIDs)
	dir := filepath.Dir(doc.File)
	var diags []Diagnostic

	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		dest := string(link.Destination)
		// Local fragment: "#section".
		if frag, ok := strings.CutPrefix(dest, "#"); ok {
			if frag == "" {
				return ast.WalkContinue, nil
			}
			if !localIDs[frag] {
				diags = append(diags, Diagnostic{
					File:    doc.File,
					Line:    lineOf(doc.Source, n),
					Check:   "anchors",
					Message: fmt.Sprintf("link %q: no heading with id %q in this file", dest, frag),
				})
			}
			return ast.WalkContinue, nil
		}
		// Cross-file fragment: only meaningful for markdown targets.
		if !shouldCheckOnDisk(dest) {
			return ast.WalkContinue, nil
		}
		target, frag, err := resolveLink(dir, dest)
		if err != nil || frag == "" {
			return ast.WalkContinue, nil
		}
		if !isMarkdown(target) {
			return ast.WalkContinue, nil
		}
		ids := doc.env.anchorsFor(target)
		if len(ids) == 0 {
			// Couldn't read target or it has no headings — leave to LinkCheck.
			return ast.WalkContinue, nil
		}
		if !ids[frag] {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    lineOf(doc.Source, n),
				Check:   "anchors",
				Message: fmt.Sprintf("link %q: %s has no heading with id %q", dest, displayPath(doc.File, target), frag),
			})
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return diags, nil
}

// DuplicateAnchorCheck flags two headings in a single document that
// produce the same slug, since deep links to such anchors are
// non-deterministic across renderers.
type DuplicateAnchorCheck struct{}

// Name implements [Check].
func (DuplicateAnchorCheck) Name() string { return "duplicate-anchors" }

// Check implements [Check].
func (DuplicateAnchorCheck) Check(doc *Document) ([]Diagnostic, error) {
	seen := make(map[string]int) // base slug -> first line
	var diags []Diagnostic

	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := headingText(h, doc.Source)
		slug := anchor.ID(text)
		if slug == "" {
			slug = "heading"
		}
		line := lineOf(doc.Source, h)
		if first, ok := seen[slug]; ok {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    line,
				Check:   "duplicate-anchors",
				Message: fmt.Sprintf("heading %q slugs to %q, already used at line %d", text, slug, first),
			})
		} else {
			seen[slug] = line
		}
		return ast.WalkSkipChildren, nil
	})
	if err != nil {
		return nil, err
	}
	return diags, nil
}

func headingText(h *ast.Heading, source []byte) string {
	var b strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		collectInlineText(&b, c, source)
	}
	return strings.TrimSpace(b.String())
}

func collectInlineText(b *strings.Builder, n ast.Node, source []byte) {
	switch t := n.(type) {
	case *ast.Text:
		b.Write(t.Segment.Value(source))
	case *ast.String:
		b.Write(t.Value)
	default:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collectInlineText(b, c, source)
		}
	}
}
