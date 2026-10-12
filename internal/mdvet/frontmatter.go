package mdvet

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
)

// FrontmatterCheck verifies the small frontmatter contract md2html relies on.
type FrontmatterCheck struct{}

// Name implements [Check].
func (FrontmatterCheck) Name() string { return "frontmatter" }

// Check implements [Check].
func (FrontmatterCheck) Check(doc *Document) ([]Diagnostic, error) {
	fm, ok, err := frontmatter(doc.Source)
	if !ok {
		return nil, nil
	}
	if err != nil {
		return []Diagnostic{{
			File:    doc.File,
			Line:    1,
			Col:     1,
			Check:   "frontmatter",
			Message: fmt.Sprintf("invalid frontmatter: %v", err),
		}}, nil
	}

	var diags []Diagnostic
	title, hasTitle := fm["title"]
	if hasTitle {
		if s, ok := title.(string); !ok || s == "" {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    1,
				Col:     1,
				Check:   "frontmatter",
				Message: "title must be a non-empty string",
			})
		}
	} else if len(fm) > 0 && !hasTitleHeading(doc) {
		// md2html titles a page by its first heading when the
		// frontmatter names none.
		diags = append(diags, Diagnostic{
			File:    doc.File,
			Line:    1,
			Col:     1,
			Check:   "frontmatter",
			Message: "title is missing, and there is no level 1 heading to take one from",
		})
	}

	if v, ok := fm["draft"]; ok {
		if _, ok := v.(bool); !ok {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    1,
				Col:     1,
				Check:   "frontmatter",
				Message: "draft must be true or false",
			})
		}
	}
	return diags, nil
}

// hasTitleHeading reports whether the body of doc, after its
// frontmatter, has a level 1 heading. The tree is parsed from the whole
// source, so a YAML comment in the frontmatter is a heading in it.
func hasTitleHeading(doc *Document) bool {
	if doc.Tree == nil {
		return false
	}
	_, body, _ := splitFrontmatter(doc.Source)
	for n := doc.Tree.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if ok && h.Level == 1 && h.Lines().Len() > 0 && h.Lines().At(0).Start >= body {
			return true
		}
	}
	return false
}

// splitFrontmatter returns the YAML frontmatter of src and the offset
// of the body that follows it. It reports false if src has no
// frontmatter, in which case the body is all of src, or if the
// frontmatter is not closed, in which case body is -1.
//
// The frontmatter is found as md2html finds it: it opens on the first
// line and closes at the next, each a line of dashes, which may have
// space around it or end in a carriage return.
func splitFrontmatter(src []byte) (fm []byte, body int, ok bool) {
	first, _, _ := bytes.Cut(src, []byte("\n"))
	if !isFrontmatterMarker(first) {
		return nil, 0, false
	}
	start := len(first) + 1
	for off := start; off < len(src); {
		line, _, found := bytes.Cut(src[off:], []byte("\n"))
		next := off + len(line) + 1
		if !found {
			next = len(src)
		}
		if isFrontmatterMarker(line) {
			return src[start:off], next, true
		}
		off = next
	}
	return nil, -1, false
}

// isFrontmatterMarker reports whether line opens or closes frontmatter.
func isFrontmatterMarker(line []byte) bool {
	line = bytes.TrimSpace(line)
	return len(line) > 0 && len(bytes.Trim(line, "-")) == 0
}

func frontmatter(src []byte) (map[string]any, bool, error) {
	fm, body, ok := splitFrontmatter(src)
	if !ok {
		if body < 0 {
			return nil, false, fmt.Errorf("missing closing marker")
		}
		return nil, false, nil
	}
	var out map[string]any
	if err := yaml.Unmarshal(fm, &out); err != nil {
		return nil, true, err
	}
	if out == nil {
		out = make(map[string]any)
	}
	return out, true, nil
}
