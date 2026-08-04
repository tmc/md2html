package components

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// An Inline is one end of an inline component: its opening tag, its
// closing tag, or a self-closing tag standing for both.
//
// Inline components are split across two nodes rather than wrapping
// their content, because goldmark parses inline text in a single pass
// with no way to open a container mid-stream. Emitting the ends
// separately is how goldmark handles raw inline HTML, and it lets the
// text between them keep its ordinary Markdown meaning.
type Inline struct {
	ast.BaseInline

	// Name is the registered component name.
	Name string

	// Attrs holds the attributes written on the opening tag. A closing
	// tag carries a copy, so the renderer can produce a suffix that
	// depends on them.
	Attrs map[string]string

	// Closing marks the node as the component's closing tag.
	Closing bool

	// SelfClosing marks a tag written as <Name/>, which renders both
	// ends at once and has no body.
	SelfClosing bool
}

// KindInlineComponent is the NodeKind of Inline.
var KindInlineComponent = ast.NewNodeKind("MD2HTMLInlineComponent")

// Kind implements ast.Node.Kind.
func (n *Inline) Kind() ast.NodeKind { return KindInlineComponent }

// Dump implements ast.Node.Dump.
func (n *Inline) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Name":    n.Name,
		"Closing": fmt.Sprint(n.Closing),
	}, nil)
}

// inlineStackContextKey tracks open inline components so a closing tag
// can find the attributes its opening tag carried.
var inlineStackContextKey = parser.NewContextKey()

func pushInline(pc parser.Context, n *Inline) {
	v, _ := pc.Get(inlineStackContextKey).([]*Inline)
	pc.Set(inlineStackContextKey, append(v, n))
}

func popInline(pc parser.Context, name string) (*Inline, bool) {
	v, _ := pc.Get(inlineStackContextKey).([]*Inline)
	if len(v) == 0 || v[len(v)-1].Name != name {
		return nil, false
	}
	top := v[len(v)-1]
	pc.Set(inlineStackContextKey, v[:len(v)-1])
	return top, true
}

// inlineParser recognises component tags inside a line of text.
type inlineParser struct {
	registry Registry
}

func (p *inlineParser) Trigger() []byte { return []byte{'<'} }

func (p *inlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	tag, ok := scanTag(line)
	if !ok {
		return nil
	}
	if tag.closing {
		open, ok := popInline(pc, tag.name)
		if !ok {
			if _, known := p.registry.Lookup(tag.name); known {
				appendError(pc, currentLine(block), "</%s> has no matching <%s>", tag.name, tag.name)
			}
			return nil
		}
		block.Advance(tag.length)
		return &Inline{Name: tag.name, Attrs: open.Attrs, Closing: true}
	}

	comp, known := p.registry.Lookup(tag.name)
	if !known {
		// Unlike a tag standing alone on a line, an unregistered name
		// inside a sentence is usually not a component at all: prose
		// carries Foo<Bar> generics and <placeholder> conventions.
		// Report nothing and leave the text to goldmark.
		return nil
	}
	if !comp.Inline {
		appendError(pc, currentLine(block),
			"<%s> is a block component and must stand alone on its own line", tag.name)
		return nil
	}
	attrs, err := parseAttrs(tag.attrs)
	if err != nil {
		appendError(pc, currentLine(block), "<%s>: %v", tag.name, err)
		return nil
	}
	if msgs := comp.validate(tag.name, attrs); len(msgs) > 0 {
		for _, msg := range msgs {
			appendError(pc, currentLine(block), "%s", msg)
		}
		return nil
	}

	block.Advance(tag.length)
	node := &Inline{Name: tag.name, Attrs: attrs, SelfClosing: tag.selfClosing}
	if !tag.selfClosing {
		pushInline(pc, node)
	}
	return node
}

// tag describes a component tag found at the start of a byte slice.
type tag struct {
	name        string
	attrs       string
	closing     bool
	selfClosing bool
	length      int // bytes the tag occupies
}

// scanTag matches a component tag at the start of line. Unlike the
// block form the tag need not fill the line, so the scan tracks quoting
// to find the ">" that ends it rather than taking the last one.
func scanTag(line []byte) (tag, bool) {
	if len(line) < 2 || line[0] != '<' {
		return tag{}, false
	}
	i := 1
	var t tag
	if line[i] == '/' {
		t.closing = true
		i++
	}
	start := i
	for i < len(line) && isNameByte(line[i]) {
		i++
	}
	t.name = string(line[start:i])
	if !isName(t.name) {
		return tag{}, false
	}

	attrStart := i
	var quote byte
	for ; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '<':
			// A new tag starts before this one ended: not a tag.
			return tag{}, false
		case c == '>':
			t.attrs = strings.TrimSpace(string(line[attrStart:i]))
			t.length = i + 1
			if rest, cut := strings.CutSuffix(t.attrs, "/"); cut {
				t.attrs, t.selfClosing = strings.TrimSpace(rest), true
			}
			if t.closing && (t.attrs != "" || t.selfClosing) {
				return tag{}, false
			}
			return t, true
		}
	}
	return tag{}, false
}
