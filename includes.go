package md2html

import (
	"cmp"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/tmc/md2html/internal/ignorefile"
	"github.com/tmc/md2html/internal/markdown/include"
)

// siteIncludes expands the Code tags of a site's pages and remembers
// what came of it: the files read, which the server watches, and the
// tags that failed, which fail a static build.
//
// A nil *siteIncludes expands nothing, and shows a tag as an error.
// That is what a caller rendering a fragment gets, since nothing has
// said which files its Markdown may read.
type siteIncludes struct {
	resolver *include.Resolver
	dir      string // directory page paths are relative to

	mu     sync.Mutex
	files  map[string]bool
	errs   map[includeError]bool
	onFile func(path string) // called, without mu, for each file not seen before
}

// An includeError is a Code tag that failed, on a line of a page.
type includeError struct {
	page string
	line int
	msg  string
}

// prepareIncludes lets the pages under sourceDir include files from the
// include root, which is sourceDir unless the configuration names
// another. Page paths are taken as relative to pagesDir.
func (s *preparedSite) prepareIncludes(sourceDir, pagesDir string) error {
	sourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("include root: %w", err)
	}
	root := sourceDir
	if s.config.IncludeRoot != "" {
		if root, err = filepath.Abs(s.config.IncludeRoot); err != nil {
			return fmt.Errorf("include root: %w", err)
		}
		if info, err := os.Stat(root); err != nil {
			return fmt.Errorf("include root: %w", err)
		} else if !info.IsDir() {
			return fmt.Errorf("include root: %s is not a directory", root)
		}
	}
	ignore, err := ignorefile.Load(sourceDir)
	if err != nil {
		return err
	}
	s.includes = &siteIncludes{
		resolver: &include.Resolver{
			Root: root,
			Excluded: func(path string) bool {
				return ignore.Excludes(path, false)
			},
			Link: repositoryLinks(sourceDir, root),
		},
		dir:   pagesDir,
		files: make(map[string]bool),
		errs:  make(map[includeError]bool),
	}
	return nil
}

// repositoryLinks returns a function giving the address, in the
// repository that the docs.json governing sourceDir names, of a file
// under root. It returns nil when no repository is named.
func repositoryLinks(sourceDir, root string) func(path string, first, last int) string {
	_, site, ok := loadDocsJSON(sourceDir, "")
	if !ok || site.RepoURL == "" {
		return nil
	}
	ref := site.Ref
	if ref == "" {
		ref = "HEAD"
	}
	// The address is of a path in the repository, which the include
	// root may be a subdirectory of.
	prefix := ""
	for dir := root; ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			if rel, err := filepath.Rel(dir, root); err == nil && rel != "." {
				prefix = filepath.ToSlash(rel) + "/"
			}
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return func(path string, first, last int) string {
		u := site.RepoURL + "/blob/" + url.PathEscape(ref) + "/" + (&url.URL{Path: prefix + path}).EscapedPath()
		switch {
		case first == 0:
		case first == last:
			u += fmt.Sprintf("#L%d", first)
		default:
			u += fmt.Sprintf("#L%d-L%d", first, last)
		}
		return u
	}
}

// expandPage returns doc, parsed from source, the page at filePath, with
// its Code tags expanded. A tag that cannot be expanded is reported at
// its line in source.
func (inc *siteIncludes) expandPage(source string, doc DocumentData, filePath string) DocumentData {
	line := 0
	if strings.HasSuffix(source, doc.Content) {
		line = strings.Count(source[:len(source)-len(doc.Content)], "\n")
	}
	doc.Content = inc.expand(doc.Content, filePath, line)
	return doc
}

// expand returns markdown, the source of the page at filePath, with its
// Code tags replaced by the files they name. A tag that cannot be
// expanded is recorded at its line in markdown plus line, the number of
// lines of the file before markdown.
func (inc *siteIncludes) expand(markdown, filePath string, line int) string {
	if !include.HasTag([]byte(markdown)) {
		return markdown
	}
	if inc == nil {
		out, _, _ := (*include.Resolver)(nil).Expand("", []byte(markdown))
		return string(out)
	}
	dir := ""
	switch {
	case filePath == "":
	case filepath.IsAbs(filePath):
		dir = filepath.Dir(filePath)
	default:
		dir = filepath.Dir(filepath.Join(inc.dir, filePath))
	}
	out, files, errs := inc.resolver.Expand(dir, []byte(markdown))

	var fresh []string
	inc.mu.Lock()
	for _, file := range files {
		if !inc.files[file] {
			inc.files[file] = true
			fresh = append(fresh, file)
		}
	}
	for _, e := range errs {
		inc.errs[includeError{filePath, line + e.Line, e.Msg}] = true
	}
	onFile := inc.onFile
	inc.mu.Unlock()
	if onFile != nil {
		for _, file := range fresh {
			onFile(file)
		}
	}
	return string(out)
}

// watch arranges for f to be called with each file a page includes
// that no page had included before.
func (inc *siteIncludes) watch(f func(path string)) {
	if inc == nil {
		return
	}
	inc.mu.Lock()
	defer inc.mu.Unlock()
	inc.onFile = f
}

// included reports whether a page has included the file at path.
func (inc *siteIncludes) included(path string) bool {
	if inc == nil {
		return false
	}
	inc.mu.Lock()
	defer inc.mu.Unlock()
	return inc.files[path]
}

// takeErrors returns the tags that failed since the last call, each as
// "page:line: message", in order.
func (inc *siteIncludes) takeErrors() []string {
	if inc == nil {
		return nil
	}
	inc.mu.Lock()
	defer inc.mu.Unlock()
	errs := slices.SortedFunc(maps.Keys(inc.errs), func(a, b includeError) int {
		return cmp.Or(cmp.Compare(a.page, b.page), cmp.Compare(a.line, b.line), cmp.Compare(a.msg, b.msg))
	})
	clear(inc.errs)
	var out []string
	for _, e := range errs {
		out = append(out, fmt.Sprintf("%s:%d: %s", e.page, e.line, e.msg))
	}
	return out
}
