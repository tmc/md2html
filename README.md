# md2html

Command md2html renders Markdown as HTML.

It can:

  - render a single Markdown file to stdout
  - serve a Markdown tree over HTTP with live reload
  - generate a static HTML tree

Basic usage:

	md2html README.md

Serve the current tree:

	md2html -http :8080

Disable live-reload file watching:

	md2html -watch=false -http :8080

Generate static output:

	md2html -html _site .

Generate static output with docs navigation:

	md2html -nav -html _site .

Generate static output with agent-readable summaries:

	md2html -html _site -llms .

Markdown supports GitHub-style alerts such as "> \[!NOTE]" and fenced admonitions such as "!!!note". Image syntax pointing at audio or video files, for example "!\[demo](demo.mp4)", renders as media controls.

MDX-style layout components are supported for `Card` and `CardGroup`:

	<CardGroup cols={2}>

	<Card title="Install" href="/install">
	Run `go install` to get started.
	</Card>

	</CardGroup>

A tag must stand alone on its line, and its body is ordinary Markdown. This is not MDX: there is no JavaScript, so `import` and `export` are unsupported and brace expressions accept only JSON scalars. Unknown component names, unknown attributes, and missing required attributes are reported as warnings and left unrendered.

Use `-format=okf` when rendering an [Open Knowledge Format](https://github.com/GoogleCloudPlatform/knowledge-catalog/tree/main/okf) bundle. It rewrites OKF root-relative Markdown links such as `/tables/orders.md` for the rendered site. It is presentation support only; validate bundles with an OKF-aware tool such as `specmd validate`.

The command is a thin wrapper around package github.com/tmc/md2html.
