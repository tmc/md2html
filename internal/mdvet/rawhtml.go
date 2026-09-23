package mdvet

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
)

// RawHTMLCheck flags raw HTML in prose, which renders to nothing.
//
// Markdown treats anything angle-bracketed as HTML, and a renderer that
// does not trust its input drops it. The failure is silent and easy to
// miss in review: "returns a List<String>" reaches the page as "returns a
// List", with no error anywhere. Backticks fix the common case, since
// nothing inside a code span is parsed as HTML.
type RawHTMLCheck struct{}

// Name implements [Check].
func (RawHTMLCheck) Name() string { return "raw-html" }

// Check implements [Check].
func (RawHTMLCheck) Check(doc *Document) ([]Diagnostic, error) {
	var diags []Diagnostic
	components := componentTags(doc.Source)

	report := func(n ast.Node, text, hint string) {
		diags = append(diags, Diagnostic{
			File:    doc.File,
			Line:    lineOf(doc.Source, n),
			Col:     1,
			Check:   "raw-html",
			Message: fmt.Sprintf("raw HTML %s is dropped when rendering; %s", quoteHTML(text), hint),
		})
	}

	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.RawHTML:
			text := rawHTMLText(node, doc.Source)
			if components[tagName(text)] {
				return ast.WalkContinue, nil
			}
			report(n, text, "wrap it in backticks to keep it as text")
		case *ast.HTMLBlock:
			text := htmlBlockText(node, doc.Source)
			if components[tagName(text)] {
				return ast.WalkSkipChildren, nil
			}
			report(n, text, "use a fenced code block or Markdown instead")
			// The segments below the block are the same HTML; one
			// diagnostic per block is enough.
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return diags, nil
}

var (
	openTag  = regexp.MustCompile(`<([A-Z][A-Za-z0-9]*)(\s[^<>]*)?/?>`)
	closeTag = regexp.MustCompile(`</([A-Z][A-Za-z0-9]*)\s*>`)
)

// componentTags returns the capitalised tag names used as components in
// source: written both open and closed, or closed on the opening tag.
//
// Those belong to ComponentCheck, which knows the registry and can say
// what is missing; reporting them here as well would double every
// diagnostic. Capitalisation alone is not enough to tell them apart:
// "List<String>" is the case this check exists for, and <String> is
// capitalised too. What distinguishes a component is that the author
// closed it.
func componentTags(source []byte) map[string]bool {
	tags := make(map[string]bool)
	for _, m := range openTag.FindAllSubmatch(source, -1) {
		if strings.HasSuffix(string(m[0]), "/>") {
			tags[string(m[1])] = true
		}
	}
	for _, m := range closeTag.FindAllSubmatch(source, -1) {
		tags[string(m[1])] = true
	}
	return tags
}

// tagName returns the element name in a raw tag, or "" if text is not one.
func tagName(text string) string {
	name := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(text), "<"), "/")
	end := strings.IndexFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if end >= 0 {
		name = name[:end]
	}
	return name
}

func rawHTMLText(n *ast.RawHTML, source []byte) string {
	var b strings.Builder
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}

func htmlBlockText(n *ast.HTMLBlock, source []byte) string {
	if n.Lines().Len() == 0 {
		return ""
	}
	seg := n.Lines().At(0)
	return string(seg.Value(source))
}

// quoteHTML renders a fragment for a diagnostic, shortened so a long
// block does not swamp the message.
func quoteHTML(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	const max = 40
	if len(s) > max {
		s = s[:max] + "…"
	}
	return fmt.Sprintf("%q", s)
}
