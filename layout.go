package md2html

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// A pageLayout says where a static build writes each page and how pages
// link to one another. Source paths are slash-separated and relative to
// the source root, as are the paths it returns.
type pageLayout struct {
	ext   string // extension of rendered files with its dot, or ""
	index string // source page rendered as the site index, if any
	dirs  bool   // write guide.md as guide/index.html, linked as guide/
}

// layout returns the page layout cfg describes.
func (cfg Config) layout() pageLayout {
	return pageLayout{
		ext:   normalizeHTMLExt(cfg.HTMLExt),
		index: cfg.Index,
		dirs:  cfg.HTMLLayout == "dir",
	}
}

func validateHTMLLayout(layout string) error {
	switch layout {
	case "", "file", "dir":
		return nil
	}
	return fmt.Errorf("invalid html-layout %q (want file or dir)", layout)
}

// file returns the file the page at source is rendered to. With
// directories, every page that is not already an index gets a directory
// of its own: guide/install.md becomes guide/install/index.html.
func (l pageLayout) file(source string) string {
	file := renderedPathForSource(source, l.ext, l.index)
	if !l.dirs || l.ext == "" || file == "" || path.Base(file) == "index"+l.ext {
		return file
	}
	return strings.TrimSuffix(file, l.ext) + "/index" + l.ext
}

// moved reports whether the page at source is rendered one directory
// below where its source sits, so that its relative references to
// files beside it need another "../".
func (l pageLayout) moved(source string) bool {
	return l.file(source) != renderedPathForSource(source, l.ext, l.index)
}

// url returns the URL of the page at source, relative to the site root.
// With directories a page is addressed by its directory, and the site
// index is "./".
func (l pageLayout) url(source string) string {
	file := l.file(source)
	if !l.dirs {
		return file
	}
	if dir := canonicalPagePath(file, l.ext); dir != "" {
		return dir
	}
	return "./"
}

// link returns the relative URL of the page at target as written in the
// page at current.
func (l pageLayout) link(current, target string) string {
	to := l.file(target)
	if to == "" {
		return ""
	}
	from := l.file(current)
	if from == "" {
		return l.url(target)
	}
	rel, err := filepath.Rel(filepath.FromSlash(path.Dir(from)), filepath.FromSlash(to))
	if err != nil {
		return to
	}
	rel = filepath.ToSlash(rel)
	if !l.dirs {
		return rel
	}
	dir, ok := strings.CutSuffix(rel, "index"+l.ext)
	if !ok {
		return rel
	}
	if dir == "" {
		return "./"
	}
	return dir
}

// root returns the relative URL of the site root as written in the
// page at source. It always ends with a slash.
func (l pageLayout) root(source string) string {
	base := assetBase(source)
	if !l.moved(source) {
		return base
	}
	if base == "./" {
		return "../"
	}
	return base + "../"
}

// relocate rewrites ref, a reference in the page at source to a file
// that is not a page, such as an image beside it. It reports whether
// the reference had to change, which it does only when the page moved.
func (l pageLayout) relocate(source, ref string) (string, bool) {
	if !l.moved(source) || !isRelativeReference(ref) {
		return ref, false
	}
	return "../" + ref, true
}

// isRelativeReference reports whether ref names something relative to
// the page it appears in, as opposed to a fragment, a root-relative
// path, or another site.
func isRelativeReference(ref string) bool {
	if ref == "" || ref[0] == '#' || ref[0] == '?' || ref[0] == '/' || strings.Contains(ref, "://") {
		return false
	}
	for _, scheme := range []string{"mailto:", "tel:", "javascript:", "data:"} {
		if strings.HasPrefix(ref, scheme) {
			return false
		}
	}
	return true
}
