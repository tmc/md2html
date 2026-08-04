// Package components implements a goldmark block extension for MDX-style
// layout components:
//
//	<CardGroup cols={2}>
//	<Card title="Install" href="/install">
//	Run `go install`.
//	</Card>
//	</CardGroup>
//
// A component tag must stand alone on its line. The tag name is matched
// against a fixed [Registry]; unknown names, unknown attributes, and
// missing required attributes are reported as parse errors rather than
// rendered. Each registered component supplies an [html/template] that
// wraps the component's Markdown body, so attribute values are escaped
// for the context they appear in.
//
// This is deliberately not MDX. There is no JavaScript evaluation, so
// import and export statements are unsupported, and attribute
// expressions in braces are limited to JSON scalars: cols={2} is
// accepted, cols={n + 1} is not.
package components
