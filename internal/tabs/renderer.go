package tabs

import (
	"fmt"
	"html"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Renderer emits ARIA-compliant tablist markup for TabGroup and Tab
// nodes. Panels are marked hidden so progressive enhancement is opt-in
// via JavaScript.
type Renderer struct{}

// RegisterFuncs implements renderer.NodeRenderer.
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindTabGroup, r.renderTabGroup)
	reg.Register(KindTab, r.renderTab)
}

func (r *Renderer) renderTabGroup(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	g := node.(*TabGroup)
	if entering {
		fmt.Fprintf(w, `<div class="md-tabs" role="tablist" data-tab-group="%s">`, html.EscapeString(g.GroupID))
		w.WriteByte('\n')
		// Emit tab buttons first so the tablist is contiguous in the
		// DOM, which matters for screen-reader focus order.
		first := true
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			t, ok := c.(*Tab)
			if !ok {
				continue
			}
			aria := ""
			tabindex := `tabindex="-1"`
			if first {
				aria = ` aria-selected="true"`
				tabindex = `tabindex="0"`
				first = false
			} else {
				aria = ` aria-selected="false"`
			}
			fmt.Fprintf(w, `  <button type="button" role="tab" id="%s-tab" aria-controls="%s"%s %s data-tab-slug="%s">%s</button>`,
				html.EscapeString(t.Slug),
				html.EscapeString(t.Slug),
				aria,
				tabindex,
				html.EscapeString(t.Slug),
				html.EscapeString(t.Label),
			)
			w.WriteByte('\n')
		}
		return ast.WalkContinue, nil
	}
	w.WriteString("</div>\n")
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTab(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	t := node.(*Tab)
	if entering {
		// All panels are emitted visible. The tabs-script adds the
		// hidden attribute on non-selected panels once it loads, so
		// no-JS readers (GitHub preview, text browsers, blocked JS)
		// still see every panel stacked instead of just the first.
		fmt.Fprintf(w, `  <div class="md-tab-panel" role="tabpanel" id="%s" aria-labelledby="%s-tab">`,
			html.EscapeString(t.Slug),
			html.EscapeString(t.Slug),
		)
		w.WriteByte('\n')
		return ast.WalkContinue, nil
	}
	w.WriteString("  </div>\n")
	return ast.WalkContinue, nil
}
