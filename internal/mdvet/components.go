package mdvet

import (
	"strings"

	"github.com/tmc/md2html/internal/markdown/components"
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
	return components.DefaultRegistry
}
