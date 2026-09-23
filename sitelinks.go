package md2html

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// siteLinks resolves root-absolute links such as "/docs/quickstart" to the
// source page that renders them. Docs written for a hosted site link to
// each other that way, which works on the site's own domain but not in a
// static build published under a prefix, such as a GitHub Pages project
// site. A static build rewrites such links relative to the page, like
// links to .md files.
//
// Paths are relative to the site root: the directory holding docs.json
// when one covers the tree, since that is what the published site's root
// URL maps to, and the source directory otherwise.
type siteLinks struct {
	// prefixes lists, most specific first, the source directory's path
	// below each candidate site root ("" when they are the same).
	prefixes []string
	// pages holds the slash-separated source paths the build renders.
	pages map[string]bool
}

// newSiteLinks returns a resolver for the pages in files, which are
// relative to sourceDir.
func newSiteLinks(sourceDir string, files []markdownFile) *siteLinks {
	l := &siteLinks{pages: make(map[string]bool, len(files))}
	for _, f := range files {
		l.pages[filepath.ToSlash(f.RelPath)] = true
	}
	if abs, err := filepath.Abs(sourceDir); err == nil {
		if siteDir, ok := findDocsJSON(abs); ok {
			if rel, err := filepath.Rel(siteDir, abs); err == nil && rel != "." {
				l.prefixes = append(l.prefixes, filepath.ToSlash(rel))
			}
		}
	}
	l.prefixes = append(l.prefixes, "")
	return l
}

// resolve returns the source path of the page href names, relative to the
// source directory, and the query and fragment to carry over. It reports
// false for anything that is not a root-absolute link to a rendered page.
func (l *siteLinks) resolve(href string) (source, suffix string, ok bool) {
	if l == nil || !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
		return "", "", false
	}
	base, suffix := splitLinkSuffix(href)
	p, err := url.PathUnescape(base)
	if err != nil {
		return "", "", false
	}
	rel := strings.Trim(path.Clean(p), "/")
	for _, prefix := range l.prefixes {
		r := rel
		if prefix != "" {
			var found bool
			if r, found = strings.CutPrefix(rel, prefix+"/"); !found {
				if rel != prefix {
					continue
				}
				r = ""
			}
		}
		for _, c := range pageCandidates(r) {
			if l.pages[c] {
				return c, suffix, true
			}
		}
	}
	return "", "", false
}

// pageCandidates lists the source paths that could render at the site
// path rel, in the order they are tried.
func pageCandidates(rel string) []string {
	if rel == "" {
		return indexNames
	}
	switch path.Ext(rel) {
	case ".md", ".markdown":
		return []string{rel}
	}
	stem := strings.TrimSuffix(rel, path.Ext(rel))
	out := []string{rel + ".md", rel + ".markdown"}
	if stem != rel {
		out = append(out, stem+".md", stem+".markdown")
	}
	for _, name := range indexNames {
		out = append(out, path.Join(rel, name))
	}
	return out
}
