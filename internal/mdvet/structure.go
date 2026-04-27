package mdvet

import (
	"fmt"

	"github.com/yuin/goldmark/ast"
)

// CodeFenceLangCheck flags fenced code blocks that omit a language tag.
// Languageless fences are valid Markdown but defeat syntax highlighting
// and make code less greppable; treating them as a vet warning helps
// catch them in review.
type CodeFenceLangCheck struct{}

// Name implements [Check].
func (CodeFenceLangCheck) Name() string { return "code-fence-lang" }

// Check implements [Check].
func (CodeFenceLangCheck) Check(doc *Document) ([]Diagnostic, error) {
	var diags []Diagnostic
	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fc, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		if len(fc.Language(doc.Source)) > 0 {
			return ast.WalkContinue, nil
		}
		diags = append(diags, Diagnostic{
			File:    doc.File,
			Line:    lineOf(doc.Source, fc),
			Check:   "code-fence-lang",
			Message: "fenced code block has no language tag",
		})
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return diags, nil
}

// HeadingSkipCheck flags heading-level jumps (e.g. h1 -> h3) and
// documents containing more than one h1. Both patterns break generated
// tables of contents and accessibility tree traversal.
type HeadingSkipCheck struct{}

// Name implements [Check].
func (HeadingSkipCheck) Name() string { return "heading-skip" }

// Check implements [Check].
func (HeadingSkipCheck) Check(doc *Document) ([]Diagnostic, error) {
	var diags []Diagnostic
	prev := 0
	h1s := 0
	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		if h.Level == 1 {
			h1s++
			if h1s == 2 {
				diags = append(diags, Diagnostic{
					File:    doc.File,
					Line:    lineOf(doc.Source, h),
					Check:   "heading-skip",
					Message: "second h1 in document; expected at most one",
				})
			}
		}
		if prev != 0 && h.Level > prev+1 {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    lineOf(doc.Source, h),
				Check:   "heading-skip",
				Message: fmt.Sprintf("heading level jumps from h%d to h%d", prev, h.Level),
			})
		}
		prev = h.Level
		return ast.WalkSkipChildren, nil
	})
	if err != nil {
		return nil, err
	}
	return diags, nil
}
