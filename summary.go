package md2html

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// NavItem represents a single navigation entry from SUMMARY.md.
type NavItem struct {
	Title    string     `json:"title"`
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
	// Remove .md extension and add htmlExt
	url := strings.TrimSuffix(path, ".md")
	url = strings.TrimSuffix(url, ".markdown")

	// Handle README -> index or directory
	if strings.HasSuffix(url, "/README") {
		url = strings.TrimSuffix(url, "/README")
		if url == "" {
			url = "."
		}
	} else if url == "README" {
		url = "."
	}

	// Add extension if specified
	if htmlExt != "" && url != "." {
		url += htmlExt
	}

	return url
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
