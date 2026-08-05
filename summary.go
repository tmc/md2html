package md2html

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// NavItem represents a single navigation entry from SUMMARY.md.
type NavItem struct {
	Title    string     `json:"title"`
	Icon     string     `json:"icon,omitempty"`     // Icon name from page frontmatter
	Path     string     `json:"path"`               // Source file path (e.g., "getting-started/installation.md")
	URL      string     `json:"url"`                // Rendered URL (with htmlExt applied)
	Level    int        `json:"level"`              // Nesting depth (0 = top level)
	Children []*NavItem `json:"children,omitempty"` // Nested items
	IsGroup  bool       `json:"isGroup"`            // True for ## section headers
	IsSep    bool       `json:"isSep"`              // True for --- separators
}

// Navigation represents the full navigation tree parsed from SUMMARY.md.
type Navigation struct {
	Items  []*NavItem          `json:"items"` // Top-level items
	ByPath map[string]*NavItem `json:"-"`     // Quick lookup by path
	Flat   []*NavItem          `json:"-"`     // Flattened for prev/next (excludes groups/seps)
}

// NavContext provides navigation state for a specific page.
type NavContext struct {
	Items      []*NavItem // Full navigation tree
	Current    *NavItem   // Current page (nil if not found)
	Prev       *NavItem   // Previous page (nil if first or not found)
	Next       *NavItem   // Next page (nil if last or not found)
	Breadcrumb []*NavItem // Ancestor chain (root to parent)
	HasNav     bool       // True if navigation was loaded
}

// linkPattern matches markdown links: [Title](path.md)
var linkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// LoadSummary loads and parses SUMMARY.md from a directory.
func (l *Loader) LoadSummary(path string) (*Navigation, error) {
	content, err := l.readFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSummary(string(content), l.htmlExt)
}

// ParseSummary parses SUMMARY.md content into a Navigation structure.
func ParseSummary(content, htmlExt string) (*Navigation, error) {
	nav := &Navigation{
		ByPath: make(map[string]*NavItem),
	}

	lines := strings.Split(content, "\n")
	var items []*NavItem

	for _, line := range lines {
		item := parseSummaryLine(line, htmlExt)
		if item != nil {
			items = append(items, item)
		}
	}

	// Build tree from flat items
	nav.Items = buildNavTree(items)

	// Build lookup map and flat list
	nav.buildIndexes()

	return nav, nil
}

// parseSummaryLine parses a single line from SUMMARY.md.
func parseSummaryLine(line, htmlExt string) *NavItem {
	trimmed := strings.TrimSpace(line)

	// Skip empty lines and title
	if trimmed == "" || strings.HasPrefix(trimmed, "# ") {
		return nil
	}

	// Section header: ## Getting Started
	if strings.HasPrefix(trimmed, "## ") {
		return &NavItem{
			Title:   strings.TrimPrefix(trimmed, "## "),
			IsGroup: true,
			Level:   0,
		}
	}

	// Separator: ---
	if trimmed == "---" || (len(trimmed) >= 3 && strings.Trim(trimmed, "-") == "") {
		return &NavItem{
			IsSep: true,
			Level: 0,
		}
	}

	// List item with link: * [Title](path.md) or - [Title](path.md)
	if strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "- ") {
		// Calculate indent level from original line
		indent := countIndent(line)
		level := indent / 2 // 2 spaces per level

		match := linkPattern.FindStringSubmatch(trimmed)
		if match != nil {
			path := match[2]
			url := pathToURL(path, htmlExt)
			return &NavItem{
				Title: match[1],
				Path:  path,
				URL:   url,
				Level: level,
			}
		}
	}

	return nil
}

// countIndent counts leading spaces in a line.
func countIndent(line string) int {
	count := 0
	for _, ch := range line {
		if ch == ' ' {
			count++
		} else if ch == '\t' {
			count += 2
		} else {
			break
		}
	}
	return count
}

// pathToURL converts a markdown path to a URL.
func pathToURL(path, htmlExt string) string {
	return renderedPathForSource(path, htmlExt, "")
}

// buildNavTree converts a flat list of items into a nested tree.
func buildNavTree(items []*NavItem) []*NavItem {
	if len(items) == 0 {
		return nil
	}

	var roots []*NavItem
	var stack []*NavItem

	for _, item := range items {
		// Groups and separators are always at root level
		if item.IsGroup || item.IsSep {
			roots = append(roots, item)
			stack = nil // Reset stack after group/sep
			continue
		}

		// Pop stack until we find the right parent level
		for len(stack) > 0 && stack[len(stack)-1].Level >= item.Level {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			// Top-level item
			roots = append(roots, item)
		} else {
			// Child of current stack top
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, item)
		}

		// Push current item if it's not a group/sep (could have children)
		if !item.IsGroup && !item.IsSep {
			stack = append(stack, item)
		}
	}

	return roots
}

// buildIndexes builds the ByPath map and Flat list.
func (n *Navigation) buildIndexes() {
	n.ByPath = make(map[string]*NavItem)
	n.Flat = nil

	var walk func(items []*NavItem)
	walk = func(items []*NavItem) {
		for _, item := range items {
			if item.Path != "" {
				n.ByPath[item.Path] = item
				n.Flat = append(n.Flat, item)
			}
			if len(item.Children) > 0 {
				walk(item.Children)
			}
		}
	}
	walk(n.Items)
}

// ForPage returns navigation context for a specific page path.
func (n *Navigation) ForPage(currentPath string) *NavContext {
	if n == nil {
		return &NavContext{HasNav: false}
	}

	ctx := &NavContext{
		Items:  n.Items,
		HasNav: true,
	}

	// Normalize path for lookup
	currentPath = normalizePath(currentPath)

	// Find current page
	ctx.Current = n.ByPath[currentPath]

	// Find prev/next in flat list
	if ctx.Current != nil {
		for i, item := range n.Flat {
			if item.Path == currentPath {
				if i > 0 {
					ctx.Prev = n.Flat[i-1]
				}
				if i < len(n.Flat)-1 {
					ctx.Next = n.Flat[i+1]
				}
				break
			}
		}

		// Build breadcrumb (find ancestors)
		ctx.Breadcrumb = n.findAncestors(currentPath)
	}

	return ctx
}

// findAncestors returns the ancestor chain for a given path.
func (n *Navigation) findAncestors(path string) []*NavItem {
	var ancestors []*NavItem

	var find func(items []*NavItem, chain []*NavItem) bool
	find = func(items []*NavItem, chain []*NavItem) bool {
		for _, item := range items {
			if item.Path == path {
				ancestors = chain
				return true
			}
			if len(item.Children) > 0 {
				newChain := append(chain, item)
				if find(item.Children, newChain) {
					return true
				}
			}
		}
		return false
	}

	find(n.Items, nil)
	return ancestors
}

// normalizePath normalizes a file path for lookup.
func normalizePath(path string) string {
	// Remove leading ./
	path = strings.TrimPrefix(path, "./")
	// Remove leading /
	path = strings.TrimPrefix(path, "/")
	// Ensure .md extension for lookup
	if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".markdown") {
		// Try to find with .md
		path = path + ".md"
	}
	return path
}

// LoadNavigationFromDir loads SUMMARY.md from a directory if it exists.
func LoadNavigationFromDir(dir, htmlExt string) *Navigation {
	summaryPath := filepath.Join(dir, "SUMMARY.md")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		return nil
	}

	loader := NewLoader(dir, htmlExt)
	nav, err := loader.LoadSummary("SUMMARY.md")
	if err != nil {
		return nil
	}
	return nav
}

// AutoNavigationFromDir builds navigation from markdown files when no
// SUMMARY.md is available.
func AutoNavigationFromDir(sourceDir string) (*Navigation, error) {
	return autoNavigationFromDir(sourceDir, "")
}

// LoadNavigationOrAutoFromDir loads SUMMARY.md when present, otherwise builds
// navigation from the markdown tree.
func LoadNavigationOrAutoFromDir(sourceDir, htmlExt string) (*Navigation, error) {
	nav, _, err := navigationForDir(sourceDir, htmlExt)
	return nav, err
}

// navigationForDir resolves navigation for a source tree, preferring an
// explicit SUMMARY.md, then a Mintlify docs.json covering the tree, then
// the shape of the tree itself. It also reports the site name and colors
// when the source it used carries them.
func navigationForDir(sourceDir, htmlExt string) (*Navigation, siteInfo, error) {
	if nav := LoadNavigationFromDir(sourceDir, htmlExt); nav != nil && len(nav.Items) > 0 {
		return nav, siteInfo{}, nil
	}
	if nav, site, ok := loadDocsJSON(sourceDir, htmlExt); ok {
		return nav, site, nil
	}
	nav, err := autoNavigationFromDir(sourceDir, htmlExt)
	return nav, siteInfo{}, err
}

type autoNavFile struct {
	relPath         string
	dir             string
	base            string
	title           string
	icon            string
	weight          int
	hasWeight       bool
	sidebarPosition int
	hasSidebar      bool
	prefix          int
	hasPrefix       bool
	isLanding       bool
}

func autoNavigationFromDir(sourceDir, htmlExt string) (*Navigation, error) {
	var files []autoNavFile
	err := filepath.WalkDir(sourceDir, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == sourceDir {
			return nil
		}
		base := entry.Name()
		if strings.HasPrefix(base, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if base == "output" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, name)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}
		if strings.EqualFold(filepath.ToSlash(rel), "SUMMARY.md") {
			return nil
		}
		f, err := readAutoNavFile(name, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		files = append(files, f)
		return nil
	})
	if err != nil {
		return nil, err
	}

	nav := &Navigation{ByPath: make(map[string]*NavItem)}
	nav.Items = buildAutoNavItems(files, htmlExt)
	nav.buildIndexes()
	return nav, nil
}

func readAutoNavFile(name, rel string) (autoNavFile, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return autoNavFile{}, fmt.Errorf("read markdown: %w", err)
	}
	doc, err := parseFrontmatter(string(data))
	if err != nil {
		doc = DocumentData{Content: string(data), Frontmatter: map[string]any{}}
	}
	base := path.Base(rel)
	stem := strings.TrimSuffix(base, path.Ext(base))
	prefix, hasPrefix, cleanStem := splitNumericPrefix(stem)
	return autoNavFile{
		relPath:         rel,
		dir:             path.Dir(rel),
		base:            base,
		title:           autoNavTitle(cleanStem, doc),
		icon:            navIcon(doc),
		weight:          frontmatterInt(doc.Frontmatter, "weight"),
		hasWeight:       hasFrontmatterInt(doc.Frontmatter, "weight"),
		sidebarPosition: frontmatterInt(doc.Frontmatter, "sidebar_position"),
		hasSidebar:      hasFrontmatterInt(doc.Frontmatter, "sidebar_position"),
		prefix:          prefix,
		hasPrefix:       hasPrefix,
		isLanding:       isLandingFile(base),
	}, nil
}

func buildAutoNavItems(files []autoNavFile, htmlExt string) []*NavItem {
	byDir := make(map[string][]autoNavFile)
	landing := make(map[string]autoNavFile)
	for _, f := range files {
		if f.dir == "." {
			f.dir = ""
		}
		if f.isLanding {
			if old, ok := landing[f.dir]; !ok || landingLess(f, old) {
				landing[f.dir] = f
			}
			continue
		}
		byDir[f.dir] = append(byDir[f.dir], f)
	}

	var dirs []string
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	var root []autoNavFile
	var items []*NavItem
	for _, dir := range dirs {
		if dir == "" {
			root = byDir[dir]
			continue
		}
		pages := byDir[dir]
		sortAutoNavFiles(pages)
		group := &NavItem{Title: autoNavGroupTitle(dir, landing), IsGroup: true, Level: 0}
		for _, f := range pages {
			group.Children = append(group.Children, autoNavItem(f, htmlExt, 1))
		}
		items = append(items, group)
	}
	sortAutoNavItems(items)
	sortAutoNavFiles(root)
	for _, f := range root {
		items = append(items, autoNavItem(f, htmlExt, 0))
	}
	return items
}

func autoNavItem(f autoNavFile, htmlExt string, level int) *NavItem {
	return &NavItem{
		Title: f.title,
		Icon:  f.icon,
		Path:  f.relPath,
		URL:   pathToURL(f.relPath, htmlExt),
		Level: level,
	}
}

func autoNavTitle(stem string, doc DocumentData) string {
	if s := firstFrontmatterString(doc.Frontmatter, "title"); s != "" {
		return s
	}
	if h := firstHeading(doc.Content); h != "" {
		return h
	}
	return titleWords(stem)
}

// iconName matches the icon names md2html is willing to carry into the
// rendered page. Icon sets name their glyphs in lowercase kebab-case
// ("rocket", "graduation-cap"); anything else is dropped rather than
// escaped, since an unusable name only means the item has no icon.
var iconName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// navIcon reports the icon a page names in its frontmatter. The name is
// not resolved to a glyph here: the nav records what the page asked for
// and leaves the drawing to the stylesheet.
func navIcon(doc DocumentData) string {
	s := strings.TrimSpace(firstFrontmatterString(doc.Frontmatter, "icon"))
	if !iconName.MatchString(s) {
		return ""
	}
	return s
}

func autoNavGroupTitle(dir string, landing map[string]autoNavFile) string {
	if f, ok := landing[dir]; ok && f.title != "" {
		return f.title
	}
	return titleWords(path.Base(dir))
}

func sortAutoNavFiles(files []autoNavFile) {
	sort.Slice(files, func(i, j int) bool {
		return autoNavLess(files[i], files[j])
	})
}

func sortAutoNavItems(items []*NavItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Title < items[j].Title
	})
}

func autoNavLess(a, b autoNavFile) bool {
	if a.hasWeight != b.hasWeight {
		return a.hasWeight
	}
	if a.hasWeight && a.weight != b.weight {
		return a.weight < b.weight
	}
	if a.hasSidebar != b.hasSidebar {
		return a.hasSidebar
	}
	if a.hasSidebar && a.sidebarPosition != b.sidebarPosition {
		return a.sidebarPosition < b.sidebarPosition
	}
	if a.hasPrefix != b.hasPrefix {
		return a.hasPrefix
	}
	if a.hasPrefix && a.prefix != b.prefix {
		return a.prefix < b.prefix
	}
	return a.relPath < b.relPath
}

func landingLess(a, b autoNavFile) bool {
	ra := landingRank(a.base)
	rb := landingRank(b.base)
	if ra != rb {
		return ra < rb
	}
	return a.relPath < b.relPath
}

func landingRank(base string) int {
	switch strings.ToLower(base) {
	case "index.md", "index.markdown":
		return 0
	case "readme.md", "readme.markdown":
		return 1
	}
	return 2
}

func isLandingFile(base string) bool {
	return landingRank(base) < 2
}

func splitNumericPrefix(stem string) (int, bool, string) {
	i := 0
	for i < len(stem) && stem[i] >= '0' && stem[i] <= '9' {
		i++
	}
	if i == 0 || i == len(stem) || (stem[i] != '-' && stem[i] != '_') {
		return 0, false, stem
	}
	n, err := strconv.Atoi(stem[:i])
	if err != nil {
		return 0, false, stem
	}
	return n, true, stem[i+1:]
}

func titleWords(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	})
	for i, f := range fields {
		if f == "" {
			continue
		}
		r := []rune(strings.ToLower(f))
		r[0] = unicode.ToUpper(r[0])
		fields[i] = string(r)
	}
	return strings.Join(fields, " ")
}

func hasFrontmatterInt(frontmatter map[string]any, key string) bool {
	_, ok := frontmatterIntValue(frontmatter, key)
	return ok
}

func frontmatterInt(frontmatter map[string]any, key string) int {
	n, _ := frontmatterIntValue(frontmatter, key)
	return n
}

func frontmatterIntValue(frontmatter map[string]any, key string) (int, bool) {
	switch v := frontmatter[key].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	default:
		return 0, false
	}
}
