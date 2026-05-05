package mdvet

import (
	"bytes"
	"fmt"

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
	} else if len(fm) > 0 {
		diags = append(diags, Diagnostic{
			File:    doc.File,
			Line:    1,
			Col:     1,
			Check:   "frontmatter",
			Message: "title is missing",
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

func frontmatter(src []byte) (map[string]any, bool, error) {
	if !bytes.HasPrefix(src, []byte("---\n")) {
		return nil, false, nil
	}
	rest := src[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return nil, false, fmt.Errorf("missing closing marker")
	}
	var out map[string]any
	if err := yaml.Unmarshal(rest[:end], &out); err != nil {
		return nil, true, err
	}
	if out == nil {
		out = make(map[string]any)
	}
	return out, true, nil
}
