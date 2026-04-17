// Package tabs implements a goldmark block extension for tabbed code
// groups using the fenced-div syntax
//
//	::: tabs <group-id>
//	::: tab <Label>
//	content
//	:::
//	:::
//
// It renders a single ARIA-compliant tab widget per group. The <group-id>
// plus a kebab-cased label form the deep-link slug for each tab.
package tabs

import "github.com/yuin/goldmark/ast"

// A TabGroup is a block that wraps a set of Tabs under a shared group ID.
type TabGroup struct {
	ast.BaseBlock

	// GroupID is the required identifier supplied after "::: tabs".
	GroupID string
}

// KindTabGroup is the NodeKind of TabGroup nodes.
var KindTabGroup = ast.NewNodeKind("MD2HTMLTabGroup")

// Kind implements ast.Node.Kind.
func (n *TabGroup) Kind() ast.NodeKind { return KindTabGroup }

// Dump implements ast.Node.Dump.
func (n *TabGroup) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"GroupID": n.GroupID}, nil)
}

// NewTabGroup returns a new TabGroup node.
func NewTabGroup(groupID string) *TabGroup {
	return &TabGroup{GroupID: groupID}
}

// A Tab is a single labelled panel inside a TabGroup.
type Tab struct {
	ast.BaseBlock

	// Label is the raw display label supplied after "::: tab".
	Label string

	// Slug is the deep-link slug for this tab, scoped to its group.
	// The panel's id attribute and the button's aria-controls attribute
	// both use this value.
	Slug string
}

// KindTab is the NodeKind of Tab nodes.
var KindTab = ast.NewNodeKind("MD2HTMLTab")

// Kind implements ast.Node.Kind.
func (n *Tab) Kind() ast.NodeKind { return KindTab }

// Dump implements ast.Node.Dump.
func (n *Tab) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Label": n.Label,
		"Slug":  n.Slug,
	}, nil)
}

// NewTab returns a new Tab node.
func NewTab(label, slug string) *Tab {
	return &Tab{Label: label, Slug: slug}
}
