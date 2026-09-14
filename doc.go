// Package md2html renders Markdown as HTML and serves or writes the result.
//
// The package provides three main entry points:
//
//	RenderFragment renders a single Markdown string.
//	NewLoader parses Markdown into headings, links, lists, and frontmatter.
//	Run drives the server and static-site modes used by the md2html command.
//
// Rendering uses Goldmark with GFM extensions, syntax highlighting,
// admonitions, MDX-style layout components,
// optional table-of-contents generation, and relative-link rewriting for Markdown
// source trees.
//
// For small in-process rendering, use RenderFragment:
//
//	frag, err := md2html.RenderFragment("# Hello\n", "", md2html.FragmentOptions{})
//	if err != nil {
//		return err
//	}
//	_ = frag.HTML
//
// For command-style integration, build a Config and call Run:
//
//	fs := md2html.NewFlagSet("md2html")
//	_ = fs.Parse([]string{"-http", ":8080", "README.md"})
//	cfg := md2html.ConfigFromFlags(fs)
//	err := md2html.Run(context.Background(), cfg, slog.Default(), os.Stdout, fs.Args())
//
// For navigation and structured parsing, use ParseSummary and Loader.
//
// Run is a library call: it neither changes the process working
// directory nor installs signal handlers. [Config.Chdir] names the
// directory the configuration's relative paths resolve against, so two
// configurations can be rendered at once, and cancelling the context
// stops a running server. The md2html command installs the interrupt
// handler itself.
//
// # Markdown dialect
//
// Rendering follows GFM: a single newline is a space, so paragraphs reflow
// to the viewport rather than breaking where the source was wrapped.
// Beyond GFM, the supported syntax is footnotes, GitHub alerts
// ("> [!NOTE]"), admonitions, tabs, media and component blocks, and
// Mermaid fences.
//
// Subscript ("H~2~O"), superscript ("X^2^"), and definition lists are not
// supported; a single tilde is GFM strikethrough, and the other two render
// literally.
//
// Raw HTML in Markdown is dropped by default and replaced with an HTML
// comment, because rendered pages may come from untrusted Markdown. The
// -allow-unsafe flag ([Config.AllowUnsafe]) passes it through and also
// enables attribute syntax and local link rewriting inside HTML blocks.
//
// Mermaid and MathJax are loaded from a CDN, so pages containing diagrams
// or math need network access on first view. When a loader fails the page
// shows a warning rather than silently leaving the source unrendered.
//
// # Compatibility
//
// The stable public API is [Config], [ConfigFromFlags], [NewFlagSet],
// [Run], [RenderFragment], [FragmentOptions], [Fragment], [FragmentStyles],
// [NewLoader], [Loader], [MarkdownDoc], [Navigation], [NavContext],
// [ParseSummary], [GitVersion], and [GitVersionManager]. Changes to
// exported names, method signatures, or exported struct fields follow
// semantic versioning.
//
// The template data passed to bundled and user templates is also part of
// the compatibility contract. Fields documented on [RenderOptions] and
// [DocumentData], plus the derived top-level template fields checked by
// the contract tests, must not be renamed, removed, or retyped without a
// deliberate compatibility decision.
package md2html
