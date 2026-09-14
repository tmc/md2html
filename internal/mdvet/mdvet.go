package mdvet

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tmc/md2html/internal/anchor"
	"github.com/tmc/md2html/internal/markdown/components"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Diagnostic is a single vet finding.
type Diagnostic struct {
	File    string // markdown file the finding came from
	Line    int    // 1-based source line, 0 if unknown
	Col     int    // 1-based source column, 0 if unknown
	Check   string // name of the check that produced the diagnostic
	Message string // human-readable description
}

// String formats the diagnostic in "file:line:col: [check] message" form.
func (d Diagnostic) String() string {
	line := d.Line
	col := d.Col
	if line == 0 {
		line = 1
	}
	if col == 0 {
		col = 1
	}
	return fmt.Sprintf("%s:%d:%d: [%s] %s", d.File, line, col, d.Check, d.Message)
}

// Document is a parsed Markdown file passed to each [Check].
type Document struct {
	File   string   // file path as given to Run
	Source []byte   // raw file bytes
	Tree   ast.Node // parsed AST root
	env    *env     // shared per-Run state for cross-file lookups

	// ctx holds the diagnostics extensions recorded while parsing.
	ctx parser.Context
}

// Check is a single vet rule.
type Check interface {
	// Name returns the short identifier used to enable or disable the check.
	Name() string
	// Check inspects a parsed document and returns any diagnostics it finds.
	Check(doc *Document) ([]Diagnostic, error)
}

// AllChecks returns the default set of checks.
func AllChecks() []Check {
	return []Check{
		NavigationCheck{},
		FrontmatterCheck{},
		AssetsCheck{},
		LinkCheck{},
		ImageCheck{},
		AnchorCheck{},
		DuplicateAnchorCheck{},
		ReferenceDefCheck{},
		CodeFenceLangCheck{},
		HeadingSkipCheck{},
		CaseCheck{},
		RawHTMLCheck{},
		ComponentCheck{},
		IconCheck{},
		GitIgnoredCheck{},
	}
}

// SelectChecks returns the subset of checks whose names appear in names.
// An empty names slice returns all checks. Unknown names produce an error.
func SelectChecks(checks []Check, names []string) ([]Check, error) {
	if len(names) == 0 {
		return checks, nil
	}
	byName := make(map[string]Check, len(checks))
	for _, c := range checks {
		byName[c.Name()] = c
	}
	out := make([]Check, 0, len(names))
	for _, n := range names {
		c, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("unknown check %q", n)
		}
		out = append(out, c)
	}
	return out, nil
}

// Run applies checks to every Markdown file reachable from paths and
// returns the collected diagnostics sorted by file and line.
//
// A path may be a file or a directory; directories are walked
// recursively for files matching .md or .markdown (case-insensitive).
func Run(paths []string, checks []Check) ([]Diagnostic, error) {
	return RunSite(paths, checks, Site{})
}

// RunSite is [Run] with site knowledge: it lets checks resolve links
// written as rendered URLs ("/docs/quickstart") back to the source file
// that produces them. See [Site].
func RunSite(paths []string, checks []Check, site Site) ([]Diagnostic, error) {
	files, err := collectFiles(paths)
	if err != nil {
		return nil, err
	}
	e := newEnv(site)
	assetMode := hasCheck(checks, "assets")
	registry := componentRegistry(checks)
	var diags []Diagnostic
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		tree, pc := parseTreeWith(src, registry)
		doc := &Document{
			File:   f,
			Source: src,
			Tree:   tree,
			env:    e,
			ctx:    pc,
		}
		for _, c := range checks {
			if assetMode && legacyAssetCheck(c.Name()) {
				continue
			}
			ds, err := c.Check(doc)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", f, c.Name(), err)
			}
			diags = append(diags, ds...)
		}
	}
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].File != diags[j].File {
			return diags[i].File < diags[j].File
		}
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Message < diags[j].Message
	})
	return diags, nil
}

func hasCheck(checks []Check, name string) bool {
	for _, c := range checks {
		if c.Name() == name {
			return true
		}
	}
	return false
}

func legacyAssetCheck(name string) bool {
	switch name {
	case "links", "images", "anchors":
		return true
	}
	return false
}

// parseTree parses Markdown source with the same options used by md2html
// when rendering, so checks see the same AST shape. Registering the
// components extension also puts links and images inside component
// bodies in reach of the checks that look for them.
//
// The parser context is returned alongside the tree because extensions
// record their diagnostics there during parsing.
func parseTreeWith(source []byte, reg components.Registry) (ast.Node, parser.Context) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			components.Extender{Registry: reg},
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	pc := parser.NewContext(parser.WithIDs(anchor.NewIDs()))
	return md.Parser().Parse(text.NewReader(source), parser.WithContext(pc)), pc
}

// parseTree parses source with the built-in components, for callers
// that only need the tree.
func parseTree(source []byte) ast.Node {
	tree, _ := parseTreeWith(source, components.DefaultRegistry())
	return tree
}

func collectFiles(paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if isMarkdown(path) {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func isMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// env holds per-Run state shared across checks: cached anchor sets for
// markdown files referenced by other documents, and a directory listing
// cache used by [CaseCheck].
type env struct {
	anchors map[string]map[string]bool // file -> set of heading IDs
	dirs    map[string]map[string]bool // dir -> set of entry names (as on disk)
	configs map[string]bool            // docs.json files already checked
	site    Site                       // how rendered URLs map back to sources
	git     *gitIgnorer                // git exclusion lookups, memoized
}

func newEnv(site Site) *env {
	return &env{
		anchors: make(map[string]map[string]bool),
		dirs:    make(map[string]map[string]bool),
		configs: make(map[string]bool),
		site:    site,
		git:     newGitIgnorer(),
	}
}

// anchorsFor returns the set of heading IDs in file. The result is
// cached. Returns nil with no error if the file cannot be read or
// parsed; callers treat that as "no anchors known" and skip checks
// rather than producing spurious diagnostics.
func (e *env) anchorsFor(file string) map[string]bool {
	if got, ok := e.anchors[file]; ok {
		return got
	}
	set := make(map[string]bool)
	src, err := os.ReadFile(file)
	if err != nil {
		e.anchors[file] = set
		return set
	}
	tree := parseTree(src)
	collectHeadingIDs(tree, set)
	e.anchors[file] = set
	return set
}

func collectHeadingIDs(tree ast.Node, into map[string]bool) {
	_ = ast.Walk(tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		if id, ok := h.AttributeString("id"); ok {
			if b, ok := id.([]byte); ok {
				into[string(b)] = true
			}
		}
		return ast.WalkContinue, nil
	})
}

// dirEntries returns the lowercase->actual-name map for directory dir.
// It is used to detect case-mismatched links on case-sensitive
// filesystems where a developer's macOS box would resolve them.
func (e *env) dirEntries(dir string) map[string]bool {
	if got, ok := e.dirs[dir]; ok {
		return got
	}
	set := make(map[string]bool)
	entries, err := os.ReadDir(dir)
	if err != nil {
		e.dirs[dir] = set
		return set
	}
	for _, ent := range entries {
		set[ent.Name()] = true
	}
	e.dirs[dir] = set
	return set
}

// lineOf returns the 1-based line number of node n in source. Inline
// nodes don't carry segment information directly, so we walk up to the
// nearest block ancestor and use its first line.
func lineOf(source []byte, n ast.Node) int {
	for cur := n; cur != nil; cur = cur.Parent() {
		if cur.Type() != ast.TypeBlock {
			continue
		}
		if cur.Lines() == nil || cur.Lines().Len() == 0 {
			continue
		}
		seg := cur.Lines().At(0)
		if seg.Start < 0 || seg.Start > len(source) {
			return 0
		}
		line := 1
		for i := 0; i < seg.Start; i++ {
			if source[i] == '\n' {
				line++
			}
		}
		return line
	}
	return 0
}

// displayPath formats target relative to the directory of file when
// possible, falling back to target as given.
func displayPath(file, target string) string {
	rel, err := filepath.Rel(filepath.Dir(file), target)
	if err != nil {
		return target
	}
	return rel
}
