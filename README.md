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

Generate static output:

	md2html -html _site .

The command is a thin wrapper around package github.com/tmc/md2html.
