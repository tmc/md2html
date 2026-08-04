package tabs

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Extender registers the tabs block parser and renderer on a goldmark
// Markdown instance.
type Extender struct{}

// Extend implements goldmark.Extender.
func (Extender) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(
		parser.WithBlockParsers(
			// tabParser must run first so that inside a Group it can
			// claim "::: tab" before tabGroupParser sees it. Outside a
			// Group it falls through and tabGroupParser handles
			// "::: tabs <id>".
			util.Prioritized(&tabParser{}, 200),
			util.Prioritized(&tabGroupParser{}, 210),
		),
	)
	md.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&Renderer{}, 200),
		),
	)
}
