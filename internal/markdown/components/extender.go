package components

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Extender registers the component block parser and renderer on a
// goldmark Markdown instance.
type Extender struct {
	// Registry supplies the component contracts. A zero Extender uses
	// [DefaultRegistry].
	Registry Registry

	// Icons resolves an icon name to its markup for templates that call
	// [Data.Icon]. A nil Icons leaves every icon empty.
	Icons IconFunc
}

// Extend implements goldmark.Extender.
func (e Extender) Extend(md goldmark.Markdown) {
	reg := e.Registry
	if reg == nil {
		reg = DefaultRegistry()
	}
	md.Parser().AddOptions(
		parser.WithBlockParsers(
			// Priority 100 beats goldmark's HTML block parser at 900,
			// which would otherwise claim the tag line first.
			util.Prioritized(&blockParser{registry: reg}, 100),
		),
		parser.WithInlineParsers(
			// Ahead of goldmark's raw inline HTML parser at 500.
			util.Prioritized(&inlineParser{registry: reg}, 100),
		),
	)
	md.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&Renderer{registry: reg, icons: e.Icons}, 100),
		),
	)
}
