package components

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// contentPlaceholder marks where a component's Markdown body belongs in
// its template output. Goldmark renders block children by walking the
// tree, so the wrapper has to be written in two pieces: the template is
// executed once with the placeholder standing in for the body, then
// split around it.
const contentPlaceholder = "\x00md2html:component-content\x00"

// Renderer renders component nodes using the templates in its registry.
type Renderer struct {
	registry Registry
}

// RegisterFuncs implements renderer.NodeRenderer.
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindComponent, r.render)
	reg.Register(KindInlineComponent, r.renderInline)
}

func (r *Renderer) renderInline(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*Inline)
	prefix, suffix, err := r.split(n.Name, n.Attrs)
	if err != nil {
		return ast.WalkStop, err
	}
	switch {
	case n.SelfClosing:
		w.WriteString(prefix)
		w.WriteString(suffix)
	case n.Closing:
		w.WriteString(suffix)
	default:
		w.WriteString(prefix)
	}
	return ast.WalkContinue, nil
}

// split executes a component template around its body placeholder,
// returning the markup that precedes and follows the body.
func (r *Renderer) split(name string, attrs map[string]string) (prefix, suffix string, err error) {
	comp, ok := r.registry.Lookup(name)
	if !ok {
		// The parser only builds nodes for registered names, so this
		// means the registry changed between parse and render.
		return "", "", fmt.Errorf("components: <%s> is not registered", name)
	}
	var buf bytes.Buffer
	data := Data{Attrs: attrs, Content: template.HTML(contentPlaceholder)}
	if err := comp.Template.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("components: render <%s>: %w", name, err)
	}
	out := buf.String()
	prefix, suffix, found := strings.Cut(out, contentPlaceholder)
	if !found {
		return "", "", fmt.Errorf("components: template for <%s> does not reference .Content", name)
	}
	if strings.Contains(suffix, contentPlaceholder) {
		return "", "", fmt.Errorf("components: template for <%s> references .Content more than once", name)
	}
	return prefix, suffix, nil
}

func (r *Renderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Node)
	if !entering {
		w.WriteString(n.suffix)
		return ast.WalkContinue, nil
	}

	prefix, suffix, err := r.split(n.Name, n.Attrs)
	if err != nil {
		return ast.WalkStop, err
	}
	if n.SelfClosing {
		w.WriteString(prefix)
		w.WriteString(suffix)
		return ast.WalkSkipChildren, nil
	}
	w.WriteString(prefix)
	n.suffix = suffix
	return ast.WalkContinue, nil
}
