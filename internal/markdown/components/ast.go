package components

import "github.com/yuin/goldmark/ast"

// A Node is a single component instance parsed from a tag pair or a
// self-closing tag.
type Node struct {
	ast.BaseBlock

	// Name is the registered component name, as written in the source.
	Name string

	// Attrs holds the parsed attribute values keyed by attribute name.
	// Values are always strings; brace expressions are converted to
	// their JSON scalar text.
	Attrs map[string]string

	// SelfClosing reports whether the tag was written as <Name/> and so
	// has no Markdown body.
	SelfClosing bool

	// OpenLine is the 1-based source line of the opening tag, used to
	// locate an unclosed component.
	OpenLine int

	// indent is the column of the opening tag. Body lines written one
	// level deeper than the tag, as JSX convention has them, are
	// stripped back by this much plus one level so they parse as
	// ordinary Markdown rather than indented code.
	indent int

	// closed records that a matching closing tag was consumed, so that
	// Close can distinguish a well-formed component from one left open
	// at end of input.
	closed bool

	// suffix holds the template output that follows the body, filled in
	// when the renderer enters the node and written when it leaves.
	suffix string
}

// KindComponent is the NodeKind of Node.
var KindComponent = ast.NewNodeKind("MD2HTMLComponent")

// Kind implements ast.Node.Kind.
func (n *Node) Kind() ast.NodeKind { return KindComponent }

// Dump implements ast.Node.Dump.
func (n *Node) Dump(source []byte, level int) {
	m := map[string]string{"Name": n.Name}
	for k, v := range n.Attrs {
		m["attr:"+k] = v
	}
	ast.DumpHelper(n, source, level, m, nil)
}

// NewNode returns a new component node.
func NewNode(name string, attrs map[string]string, selfClosing bool, openLine int) *Node {
	return &Node{
		Name:        name,
		Attrs:       attrs,
		SelfClosing: selfClosing,
		OpenLine:    openLine,
	}
}
