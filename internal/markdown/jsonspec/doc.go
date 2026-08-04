// Package jsonspec enriches fenced JSON code blocks whose body contains
// a type discriminator (for example "type": "ascf/hypothesis") with a
// schema badge link and a data-schema-type attribute on a wrapping div.
//
// The enrichment is opt-in at the markdown level: any ```json fence
// whose payload matches the configured discriminator pattern is wrapped.
// Fences without a discriminator fall through to the default rendering.
//
// Usage:
//
//	md := goldmark.New(
//		goldmark.WithExtensions(
//			highlighting.NewHighlighting(
//				highlighting.WithWrapperRenderer(jsonspec.WrapperRenderer(cfg)),
//			),
//			jsonspec.Extension(cfg),
//		),
//	)
//
// The extension is split in two pieces because the discriminator is
// detected by an AST transformer (needs the node), while the surrounding
// markup is emitted by a WrapperRenderer passed to the highlighting
// extension (needs to run around chroma's own output).
package jsonspec
