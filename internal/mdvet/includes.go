package mdvet

import (
	"path/filepath"

	"github.com/tmc/md2html/internal/ignorefile"
	"github.com/tmc/md2html/internal/markdown/include"
)

// IncludeCheck reports Code tags that md2html could not expand: a file
// that is missing, hidden, or outside the include root, a region the
// file does not mark, an attribute the tag does not have.
//
// It uses the resolver md2html renders with, so the two agree about
// every tag.
type IncludeCheck struct {
	// Resolver expands the tags. Nil means one like md2html's: it
	// excludes the files the site's ignore file names, and its include
	// root is Root. Unlike md2html's, it does not read docs.json, so it
	// takes any tag's link to be satisfiable.
	Resolver *include.Resolver

	// Root is the include root when Resolver is nil. Empty means the
	// site root, or the directory of the page when there is none.
	Root string
}

// Name implements Check.
func (IncludeCheck) Name() string { return "includes" }

// Check implements Check.
func (c IncludeCheck) Check(doc *Document) ([]Diagnostic, error) {
	if !include.HasTag(doc.Source) {
		return nil, nil
	}
	dir, err := filepath.Abs(filepath.Dir(doc.File))
	if err != nil {
		return nil, err
	}
	r := c.Resolver
	if r == nil {
		site := dir
		if doc.env != nil && doc.env.site.Root != "" {
			if site, err = filepath.Abs(doc.env.site.Root); err != nil {
				return nil, err
			}
		}
		root := site
		if c.Root != "" {
			root = c.Root
		}
		ignore, err := ignorefile.Load(site)
		if err != nil {
			return nil, err
		}
		r = &include.Resolver{
			Root: root,
			Excluded: func(path string) bool {
				return ignore.Excludes(path, false)
			},
			Link: func(string, int, int) string { return "-" },
		}
	}
	_, _, errs := r.Expand(dir, doc.Source)
	var diags []Diagnostic
	for _, e := range errs {
		diags = append(diags, Diagnostic{
			File:    doc.File,
			Line:    e.Line,
			Col:     1,
			Check:   c.Name(),
			Message: e.Msg,
		})
	}
	return diags, nil
}
