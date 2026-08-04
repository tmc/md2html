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
}

func (r *Renderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Node)
	if !entering {
		w.WriteString(n.suffix)
		return ast.WalkContinue, nil
	}

	comp, ok := r.registry.Lookup(n.Name)
	if !ok {
		// The parser only builds nodes for registered names, so this
		// means the registry changed between parse and render.
		return ast.WalkStop, fmt.Errorf("components: <%s> is not registered", n.Name)
	}
	var buf bytes.Buffer
	data := Data{Attrs: n.Attrs, Content: template.HTML(contentPlaceholder)}
	if err := comp.Template.Execute(&buf, data); err != nil {
		return ast.WalkStop, fmt.Errorf("components: render <%s>: %w", n.Name, err)
	}
	out := buf.String()
	prefix, suffix, found := strings.Cut(out, contentPlaceholder)
	if !found {
		return ast.WalkStop, fmt.Errorf("components: template for <%s> does not reference .Content", n.Name)
	}
	if strings.Contains(suffix, contentPlaceholder) {
		return ast.WalkStop, fmt.Errorf("components: template for <%s> references .Content more than once", n.Name)
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
