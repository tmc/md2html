package md2html

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/md2html/internal/anchor"
	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// Loader loads and parses data files (JSON, YAML, Markdown).
type Loader struct {
	baseDir string
	htmlExt string // Extension for HTML files (e.g., ".html" or "")
}

// NewLoader creates a new data loader rooted at baseDir.
func NewLoader(baseDir, htmlExt string) *Loader {
	return &Loader{
		baseDir: baseDir,
		htmlExt: htmlExt,
	}
}

// Load auto-detects file type by extension and loads accordingly.
func (l *Loader) Load(path string) (any, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return l.LoadJSON(path)
	case ".yaml", ".yml":
		return l.LoadYAML(path)
	case ".md", ".markdown":
		return l.LoadMD(path)
	default:
		// Return raw content for unknown types
		content, err := l.readFile(path)
		if err != nil {
			return nil, err
		}
		return string(content), nil
	}
}

// LoadJSON loads and parses a JSON file.
func (l *Loader) LoadJSON(path string) (any, error) {
	content, err := l.readFile(path)
	if err != nil {
		return nil, err
	}

	var data any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// LoadYAML loads and parses a YAML file.
func (l *Loader) LoadYAML(path string) (any, error) {
	content, err := l.readFile(path)
	if err != nil {
		return nil, err
	}

	var data any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// MarkdownDoc represents a parsed markdown file with extracted structure.
type MarkdownDoc struct {
	Frontmatter map[string]any // YAML frontmatter
	Content     string         // Raw markdown content
	HTML        template.HTML  // Rendered HTML
	Title       string         // First H1 or frontmatter title
	Headings    []Heading      // All headings
	Links       []Link         // All links with context
	Lists       []ListItem     // Top-level list items (with nesting)
}

// Heading represents a heading in the document.
type Heading struct {
	Level int    // 1-6
	Text  string // Heading text
	ID    string // Anchor ID
}

// Link represents a link in the document.
type Link struct {
	Text   string // Link text
	URL    string // Link URL
	Line   int    // Source line number
	Indent int    // Indentation level (for list items)
}

// ListItem represents an item in a list, potentially with nested children.
type ListItem struct {
	Text     string     // Item text (without link)
	Link     *Link      // Link if item contains one
	Indent   int        // Nesting level (0 = top level)
	Children []ListItem // Nested list items
}

// LoadMD loads and parses a markdown file into structured data.
func (l *Loader) LoadMD(path string) (*MarkdownDoc, error) {
	content, err := l.readFile(path)
	if err != nil {
		return nil, err
	}

	return l.ParseMD(string(content))
}

// ParseMD parses markdown content into a MarkdownDoc.
func (l *Loader) ParseMD(content string) (*MarkdownDoc, error) {
	doc := &MarkdownDoc{
		Content:     content,
		Frontmatter: make(map[string]any),
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
		),
	)

	ctx := parser.NewContext(parser.WithIDs(anchor.NewIDs()))
	source := []byte(content)
	reader := text.NewReader(source)
	tree := md.Parser().Parse(reader, parser.WithContext(ctx))

	if metadata := meta.Get(ctx); metadata != nil {
		doc.Frontmatter = metadata
		if title, ok := metadata["title"].(string); ok {
			doc.Title = title
		}
	}

	l.extractStructure(doc, tree, source)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, tree); err != nil {
		return nil, err
	}
	doc.HTML = template.HTML(buf.String())

	return doc, nil
}

// extractStructure walks the AST and extracts headings, links, and lists.
func (l *Loader) extractStructure(doc *MarkdownDoc, tree ast.Node, source []byte) {
	var currentIndent int
	var listStack []int // Track list nesting depth

	ast.Walk(tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if _, ok := n.(*ast.List); ok && len(listStack) > 0 {
				listStack = listStack[:len(listStack)-1]
			}
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			heading := Heading{
				Level: node.Level,
				Text:  nodeText(node, source),
				ID:    anchor.ID(nodeText(node, source)),
			}
			doc.Headings = append(doc.Headings, heading)

			// Use first H1 as title if not set from frontmatter
			if node.Level == 1 && doc.Title == "" {
				doc.Title = heading.Text
			}

		case *ast.Link:
			link := Link{
				Text:   nodeText(node, source),
				URL:    string(node.Destination),
				Indent: currentIndent,
			}
			doc.Links = append(doc.Links, link)

		case *ast.List:
			listStack = append(listStack, 0)

		case *ast.ListItem:
			if len(listStack) > 0 {
				currentIndent = len(listStack) - 1
			}
			item := l.extractListItem(node, source, currentIndent)
			if item != nil {
				doc.Lists = append(doc.Lists, *item)
			}
		}

		return ast.WalkContinue, nil
	})

	doc.Lists = buildListTree(doc.Lists)
}

// extractListItem extracts a single list item's content.
func (l *Loader) extractListItem(node *ast.ListItem, source []byte, indent int) *ListItem {
	item := &ListItem{
		Indent: indent,
	}

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if para, ok := child.(*ast.Paragraph); ok {
			for pchild := para.FirstChild(); pchild != nil; pchild = pchild.NextSibling() {
				if link, ok := pchild.(*ast.Link); ok {
					item.Link = &Link{
						Text:   nodeText(link, source),
						URL:    string(link.Destination),
						Indent: indent,
					}
					return item
				}
			}
			// No link found, use text content
			item.Text = nodeText(para, source)
		}
	}

	return item
}

// buildListTree converts flat list items into a nested tree based on indent levels.
func buildListTree(items []ListItem) []ListItem {
	if len(items) == 0 {
		return nil
	}

	var roots []ListItem
	var stack []*ListItem

	for i := range items {
		item := items[i]

		for len(stack) > 0 && stack[len(stack)-1].Indent >= item.Indent {
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			roots = append(roots, item)
			stack = append(stack, &roots[len(roots)-1])
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, item)
			stack = append(stack, &parent.Children[len(parent.Children)-1])
		}
	}

	return roots
}

// readFile reads a file relative to the loader's base directory.
func (l *Loader) readFile(path string) ([]byte, error) {
	if filepath.IsAbs(path) {
		return nil, fmt.Errorf("absolute paths are not allowed")
	}
	fullPath, err := secureJoin(l.baseDir, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(fullPath)
}

// TemplateFuncs returns template functions for data loading.
func (l *Loader) TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"load":     l.Load,
		"loadJSON": l.LoadJSON,
		"loadYAML": l.LoadYAML,
		"loadMD":   l.LoadMD,
	}
}

// nodeText returns the plain text of a node and its descendants.
//
// goldmark's ast.Node.Text is deprecated, and it read the whole source
// segment; walking the text nodes gives the same answer from the parsed
// document, which is what the headings, links, and list items recorded
// here are built from.
func nodeText(n ast.Node, source []byte) string {
	var buf []byte
	ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			buf = append(buf, t.Segment.Value(source)...)
		case *ast.String:
			buf = append(buf, t.Value...)
		}
		return ast.WalkContinue, nil
	})
	return string(buf)
}
