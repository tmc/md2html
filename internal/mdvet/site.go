package mdvet

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Site describes how the documents being checked are addressed once
// published. Docs written for a hosted site link to each other by their
// rendered URL ("/docs/quickstart", not "quickstart.md"), and without
// this those links name nothing on disk, so neither the target nor its
// "#fragment" can be checked.
//
// The zero Site resolves nothing, which is the right default for a tree
// whose links are relative.
type Site struct {
	// Base is the URL path prefix the tree is served under, such as
	// "/docs". Empty means the tree is served at the root.
	Base string

	// Root is the directory Base maps to on disk. Empty disables
	// resolution entirely.
	Root string
}

// normalizeBase cleans a prefix into "" or a rooted path with no
// trailing slash.
func normalizeBase(base string) string {
	base = strings.TrimSpace(base)
	if base == "" || base == "/" {
		return ""
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = path.Clean(base)
	if base == "/" {
		return ""
	}
	return base
}

// owns reports whether dest is a rooted URL path this site is
// responsible for: the tree's own prefix, not a link out to some other
// part of the host.
func (s Site) owns(dest string) bool {
	if s.Root == "" {
		return false
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return false
	}
	base := normalizeBase(s.Base)
	if base == "" {
		return true
	}
	return u.Path == base || strings.HasPrefix(u.Path, base+"/")
}

// resolve maps a site-absolute URL to the source file that renders it,
// along with any fragment. It reports false when s cannot resolve the
// URL (no Root configured, a different prefix, or no source file for
// that path), so callers fall back to their unresolved handling rather
// than inventing a diagnostic about a file they cannot see.
func (s Site) resolve(dest string) (file, frag string, ok bool) {
	if s.Root == "" {
		return "", "", false
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "", "", false
	}
	p, err := url.PathUnescape(u.Path)
	if err != nil || !strings.HasPrefix(p, "/") {
		return "", "", false
	}

	base := normalizeBase(s.Base)
	if base != "" {
		if p != base && !strings.HasPrefix(p, base+"/") {
			return "", "", false
		}
		p = strings.TrimPrefix(p, base)
	}
	rel := strings.Trim(path.Clean("/"+p), "/")

	for _, candidate := range sourceCandidates(rel) {
		full := filepath.Join(s.Root, filepath.FromSlash(candidate))
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, u.Fragment, true
		}
	}
	return "", "", false
}

// sourceCandidates lists the source paths that could render as the
// rendered path rel, in the order md2html itself would try them.
func sourceCandidates(rel string) []string {
	if rel == "" || rel == "." {
		return []string{"index.md", "index.markdown", "README.md"}
	}
	// A link may name the rendered file outright.
	if ext := path.Ext(rel); ext == ".md" || ext == ".markdown" {
		return []string{rel}
	}
	stem := strings.TrimSuffix(rel, path.Ext(rel))
	return []string{
		rel + ".md",
		rel + ".markdown",
		stem + ".md",
		stem + ".markdown",
		path.Join(rel, "index.md"),
		path.Join(rel, "index.markdown"),
		path.Join(rel, "README.md"),
		path.Join(rel, "SKILL.md"),
	}
}
