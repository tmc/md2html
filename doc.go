// Package md2html renders Markdown as HTML and serves or writes the result.
//
// The package provides three main entry points:
//
//	RenderFragment renders a single Markdown string.
//	NewLoader parses Markdown into headings, links, lists, and frontmatter.
//	Run drives the server and static-site modes used by the md2html command.
//
// Rendering uses Goldmark with GFM extensions, syntax highlighting, admonitions,
// optional table-of-contents generation, and relative-link rewriting for Markdown
// source trees.
//
// For small in-process rendering, use RenderFragment:
//
//	frag := md2html.RenderFragment("# Hello\n", "", md2html.FragmentOptions{})
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
package md2html
