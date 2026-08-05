package mdvet

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// LinkCheck verifies that relative links in a Markdown file resolve to
// an existing file or directory on disk. It mirrors how GitHub renders
// repository-relative links.
//
// Links are skipped when:
//   - the URL has a scheme (http, https, mailto, ...)
//   - the URL is a pure fragment ("#section")
//   - the URL is empty or starts with "//"
type LinkCheck struct{}

// Name implements [Check].
func (LinkCheck) Name() string { return "links" }

// Check implements [Check].
func (LinkCheck) Check(doc *Document) ([]Diagnostic, error) {
	return walkOnDiskRefs(doc, "links", false)
}

// ImageCheck verifies that relative image references resolve to a file
// on disk. It uses the same resolution rules as [LinkCheck].
type ImageCheck struct{}

// Name implements [Check].
func (ImageCheck) Name() string { return "images" }

// Check implements [Check].
func (ImageCheck) Check(doc *Document) ([]Diagnostic, error) {
	return walkOnDiskRefs(doc, "images", true)
}

// walkOnDiskRefs handles both LinkCheck and ImageCheck: it walks the
// AST and reports broken on-disk references. When images is true it
// processes *ast.Image nodes; otherwise *ast.Link nodes.
func walkOnDiskRefs(doc *Document, name string, images bool) ([]Diagnostic, error) {
	dir := filepath.Dir(doc.File)
	var diags []Diagnostic

	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest string
		switch node := n.(type) {
		case *ast.Link:
			if images {
				return ast.WalkContinue, nil
			}
			dest = string(node.Destination)
		case *ast.Image:
			if !images {
				return ast.WalkContinue, nil
			}
			dest = string(node.Destination)
		default:
			return ast.WalkContinue, nil
		}
		if !shouldCheckOnDisk(dest) {
			return ast.WalkContinue, nil
		}
		// A rooted path inside the site's own prefix is a page URL, not
		// a filesystem path. Resolve it to the source that renders it;
		// one that resolves to nothing is a broken link, which is a
		// sharper finding than declining to look.
		if doc.env.site.owns(dest) {
			if _, _, ok := doc.env.site.resolve(dest); !ok {
				diags = append(diags, Diagnostic{
					File:    doc.File,
					Line:    lineOf(doc.Source, n),
					Check:   name,
					Message: fmt.Sprintf("link %q: no page in this tree renders that URL", dest),
				})
			}
			return ast.WalkContinue, nil
		}
		if isAbsolutePathLink(dest) {
			// An absolute filesystem path inside a markdown link is
			// almost always a mistake (leaked worktree path, points
			// outside the repo, won't survive rendering). Refuse to
			// validate rather than statting it: a stat that succeeds
			// on the author's box and fails on CI would obscure the
			// real problem.
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    lineOf(doc.Source, n),
				Check:   name,
				Message: fmt.Sprintf("link %q: absolute path; mdvet refuses to validate", dest),
			})
			return ast.WalkContinue, nil
		}
		target, frag, err := resolveLink(dir, dest)
		if err != nil {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    lineOf(doc.Source, n),
				Check:   name,
				Message: fmt.Sprintf("invalid link %q: %v", dest, err),
			})
			return ast.WalkContinue, nil
		}
		info, err := os.Stat(target)
		if err != nil {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    lineOf(doc.Source, n),
				Check:   name,
				Message: fmt.Sprintf("link %q: %s does not exist", dest, displayPath(doc.File, target)),
			})
			return ast.WalkContinue, nil
		}
		if info.IsDir() && frag != "" && !images {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    lineOf(doc.Source, n),
				Check:   name,
				Message: fmt.Sprintf("link %q: anchor on directory target %s", dest, displayPath(doc.File, target)),
			})
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return diags, nil
}

// isAbsolutePathLink reports whether dest is an absolute filesystem
// path used as a link target (e.g. "/Users/me/notes.md", "C:\\foo").
// Such links are almost always mistakes — leaked from another
// machine's worktree, or pointing outside the repository — and should
// be flagged rather than validated against the local filesystem.
func isAbsolutePathLink(dest string) bool {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" {
		return false
	}
	p, err := url.PathUnescape(u.Path)
	if err != nil {
		return false
	}
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "/") {
		return true
	}
	// Windows drive-letter absolute path: "C:\\..." or "C:/...".
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		c := p[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return true
		}
	}
	return false
}

// shouldCheckOnDisk reports whether dest is a candidate for on-disk
// resolution. It rejects empty links, pure fragments, protocol-relative
// links, and any URL with a scheme.
func shouldCheckOnDisk(dest string) bool {
	if dest == "" {
		return false
	}
	if strings.HasPrefix(dest, "#") {
		return false
	}
	if strings.HasPrefix(dest, "//") {
		return false
	}
	u, err := url.Parse(dest)
	if err != nil {
		return false
	}
	if u.Scheme != "" {
		return false
	}
	return true
}

// errAbsolutePathLink is returned by resolveLink when the destination
// is an absolute filesystem path. Callers that want to flag such links
// use [isAbsolutePathLink] directly; everywhere else this surfaces as
// a generic error and is silently skipped.
var errAbsolutePathLink = fmt.Errorf("absolute path link")

// resolveLink turns a markdown link destination into an absolute path,
// stripping the URL fragment and decoding percent-escapes. The returned
// frag is the link's "#anchor" portion, if any. Absolute filesystem
// paths return [errAbsolutePathLink] — they are not validated against
// the local filesystem because such links almost always reflect a
// leaked path from another machine.
func resolveLink(dir, dest string) (path, frag string, err error) {
	u, err := url.Parse(dest)
	if err != nil {
		return "", "", err
	}
	frag = u.Fragment
	p, err := url.PathUnescape(u.Path)
	if err != nil {
		return "", "", err
	}
	if p == "" {
		// e.g. "?query#frag" without a path — treat as same-file.
		p = "."
	}
	if isAbsolutePathLink(dest) {
		return "", frag, errAbsolutePathLink
	}
	return filepath.Join(dir, filepath.FromSlash(p)), frag, nil
}
