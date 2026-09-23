package mdvet

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// ReferenceDefCheck flags reference-style links whose label has no
// matching definition, and definitions that no link references.
//
// Goldmark resolves valid [text][ref] links inline by the time the AST
// is built, so the AST alone can't distinguish "matched reference" from
// "inline link". To find unresolved references we re-scan the source
// for [text][label] patterns and check each label against the document
// references.
type ReferenceDefCheck struct{}

// Name implements [Check].
func (ReferenceDefCheck) Name() string { return "reference-defs" }

// refUseRE matches [text][label] and ![alt][label] reference links.
// It is intentionally simple: it doesn't try to track code spans, so
// callers should treat its results as best-effort.
var refUseRE = regexp.MustCompile(`(?m)!?\[(?:[^\]]*)\]\[([^\]]*)\]`)

// refDefRE matches link reference definitions: [label]: dest "title".
var refDefRE = regexp.MustCompile(`(?m)^\s{0,3}\[([^\]]+)\]:\s*\S`)

// shortcutRE matches collapsed/shortcut references that look like
// [label] not followed by ( or [ or :. We use it to find references
// like [foo] when [foo]: ... is defined later. Goldmark already
// resolves these, but we reuse the matcher for diagnostics.

// Check implements [Check].
func (ReferenceDefCheck) Check(doc *Document) ([]Diagnostic, error) {
	src := string(doc.Source)
	defs := make(map[string]int) // label -> 1-based line
	for _, m := range refDefRE.FindAllStringSubmatchIndex(src, -1) {
		label := normalizeRefLabel(src[m[2]:m[3]])
		if _, ok := defs[label]; !ok {
			defs[label] = lineOfOffset(doc.Source, m[0])
		}
	}

	used := make(map[string]bool)
	var diags []Diagnostic
	codes := codeRanges(doc.Tree)

	// Full reference form [text][label]: the label is what we check.
	for _, m := range refUseRE.FindAllStringSubmatchIndex(src, -1) {
		// Skip if this match lies inside a code block or code span.
		if inCodeRange(codes, m[0]) {
			continue
		}
		label := normalizeRefLabel(src[m[2]:m[3]])
		if label == "" {
			// Collapsed form [text][]: label comes from text.
			label = normalizeRefLabel(extractLinkText(src, m[0]))
			if label == "" {
				continue
			}
		}
		used[label] = true
		if _, ok := defs[label]; !ok {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    lineOfOffset(doc.Source, m[0]),
				Check:   "reference-defs",
				Message: fmt.Sprintf("undefined reference %q", label),
			})
		}
	}

	// Orphan definitions: defined but never used.
	for label, line := range defs {
		if !used[label] {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    line,
				Check:   "reference-defs",
				Message: fmt.Sprintf("unused reference definition %q", label),
			})
		}
	}
	return diags, nil
}

// normalizeRefLabel applies CommonMark label normalization: trim,
// collapse whitespace, fold to lowercase.
func normalizeRefLabel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	return strings.Join(strings.Fields(s), " ")
}

// extractLinkText returns the "text" portion of a [text][label] match
// starting at offset start in src.
func extractLinkText(src string, start int) string {
	if start >= len(src) {
		return ""
	}
	open := strings.IndexByte(src[start:], '[')
	if open < 0 {
		return ""
	}
	open += start + 1
	close := strings.IndexByte(src[open:], ']')
	if close < 0 {
		return ""
	}
	return src[open : open+close]
}

// codeRanges returns the half-open byte ranges in source covered by
// fenced/indented code blocks and inline code spans. Blocks expose
// their range via .Lines(); code spans (which are inline and panic on
// .Lines()) are derived from the segments of their *ast.Text children.
func codeRanges(tree ast.Node) [][2]int {
	var out [][2]int
	addBlock := func(n ast.Node) {
		lines := n.Lines()
		if lines == nil || lines.Len() == 0 {
			return
		}
		first := lines.At(0)
		last := lines.At(lines.Len() - 1)
		out = append(out, [2]int{first.Start, last.Stop})
	}
	_ = ast.Walk(tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.FencedCodeBlock:
			addBlock(v)
		case *ast.CodeBlock:
			addBlock(v)
		case *ast.CodeSpan:
			// Inline; .Lines() panics. Use children's segments instead.
			start, stop := -1, -1
			for c := v.FirstChild(); c != nil; c = c.NextSibling() {
				t, ok := c.(*ast.Text)
				if !ok {
					continue
				}
				if start < 0 || t.Segment.Start < start {
					start = t.Segment.Start
				}
				if t.Segment.Stop > stop {
					stop = t.Segment.Stop
				}
			}
			if start >= 0 && stop > start {
				out = append(out, [2]int{start, stop})
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return out
}

// inCodeRange reports whether offset falls in any of ranges.
func inCodeRange(ranges [][2]int, offset int) bool {
	for _, r := range ranges {
		if offset >= r[0] && offset < r[1] {
			return true
		}
	}
	return false
}

func lineOfOffset(source []byte, off int) int {
	if off < 0 || off > len(source) {
		return 0
	}
	line := 1
	for i := range off {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}
