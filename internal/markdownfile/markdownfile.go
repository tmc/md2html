// Package markdownfile names the files md2html treats as pages.
//
// Every part of md2html that walks a tree, resolves a link, or answers a
// request has to agree on which files are pages. Keeping the answer here
// stops them from drifting apart.
package markdownfile

import (
	"path"
	"path/filepath"
	"slices"
	"strings"
)

var exts = []string{".md", ".markdown", ".mdx"}

// Exts returns the page file extensions, in the order they are tried
// when a path names a page without one.
func Exts() []string {
	return slices.Clone(exts)
}

// Is reports whether name has a page file extension. The comparison
// ignores case, and name may use either path separator.
func Is(name string) bool {
	return slices.Contains(exts, strings.ToLower(filepath.Ext(name)))
}

// TrimExt returns name without its page file extension. A name with any
// other extension, or none, is returned unchanged.
func TrimExt(name string) string {
	if !Is(name) {
		return name
	}
	return name[:len(name)-len(filepath.Ext(name))]
}

// IsIndex reports whether name is a directory's index page: a page file
// whose base name is "index".
func IsIndex(name string) bool {
	return Is(name) && path.Base(filepath.ToSlash(TrimExt(name))) == "index"
}
