package mdvet

import (
	"sort"
	"strings"

	"github.com/tmc/md2html/internal/markdown/components"
	"github.com/yuin/goldmark/ast"
)

// ComponentCheck reports malformed component blocks: unregistered
// names, unknown or missing attributes, and tags left open.
//
// The zero value checks against the built-in components. A site with
// its own components must supply Registry, or every one of them is
// reported as unknown.
type ComponentCheck struct {
	// Registry holds the component contracts to check against. Nil
	// means the built-in components.
	Registry components.Registry

	// Configured records that Registry came from a components
	// directory, which lets an unknown name explain itself when the
	// directory was simply never passed.
	Configured bool
}

// Name implements Check.
func (ComponentCheck) Name() string { return "components" }

// Check implements Check. The diagnostics were recorded while the
// document was parsed, so this only formats them.
func (c ComponentCheck) Check(doc *Document) ([]Diagnostic, error) {
	if doc.ctx == nil {
		return nil, nil
	}
	var diags []Diagnostic
	for _, e := range components.Errors(doc.ctx) {
		diags = append(diags, Diagnostic{
			File:    doc.File,
			Line:    e.Line,
			Col:     1,
			Check:   c.Name(),
			Message: c.message(e),
		})
	}
	return diags, nil
}

// message explains an unknown component in terms of how the registry
// was configured, since the usual cause of a genuinely unknown name is
// a components directory that was never passed.
func (c ComponentCheck) message(e *components.ParseError) string {
	if !e.UnknownComponent {
		return e.Msg
	}
	var b strings.Builder
	b.WriteString(e.Msg)
	b.WriteString(" (")
	if !c.Configured {
		b.WriteString("no -components directory configured; ")
	}
	b.WriteString("known: ")
	b.WriteString(strings.Join(e.Known, ", "))
	b.WriteString(")")
	return b.String()
}

// componentRegistry returns the registry that Run should parse with,
// taken from the components check when it is enabled.
func componentRegistry(checks []Check) components.Registry {
	for _, c := range checks {
		if cc, ok := c.(ComponentCheck); ok && cc.Registry != nil {
			return cc.Registry
		}
	}
	return components.DefaultRegistry()
}

// destAttrs are the component attributes whose value names a
// destination, and whether that destination is an image rather than a
// link. A component contract declares attribute names but not what they
// mean, so the name is the only signal there is — and it is a reliable
// one, since a component that points somewhere spells it the way HTML
// does and its template renders it into href="" or src="".
var destAttrs = map[string]bool{
	"href": false,
	"src":  true,
}

// A componentDest is one destination written on a component tag, such
// as the href on <Card title="Go" href="/go">.
type componentDest struct {
	value string // the attribute value, as written
	image bool   // the attribute names an image rather than a link
	line  int    // 1-based line of the opening tag
}

// componentDests returns the destinations written on n, or nil when n
// is not a component. Attribute order in the source is not recorded, so
// the result is ordered by attribute name to keep diagnostics from one
// tag stable across runs.
func componentDests(n ast.Node) []componentDest {
	c, ok := n.(*components.Node)
	if !ok {
		return nil
	}
	attrs := make([]string, 0, len(c.Attrs))
	for attr := range c.Attrs {
		if _, ok := destAttrs[attr]; ok {
			attrs = append(attrs, attr)
		}
	}
	sort.Strings(attrs)
	out := make([]componentDest, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, componentDest{
			value: c.Attrs[attr],
			image: destAttrs[attr],
			line:  c.OpenLine,
		})
	}
	return out
}
