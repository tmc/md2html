// Command md2html renders Markdown as HTML.
//
// It can:
//
//   - render a single Markdown file to stdout
//   - serve a Markdown tree over HTTP with live reload
//   - generate a static HTML tree
//
// Basic usage:
//
//	md2html README.md
//
// Serve the current tree:
//
//	md2html -http :8080
//
// Disable live-reload file watching:
//
//	md2html -watch=false -http :8080
//
// Generate static output:
//
//	md2html -html _site .
//
// Generate static output with docs navigation:
//
//	md2html -nav -html _site .
//
// Generate static output with agent-readable summaries:
//
//	md2html -html _site -llms .
//
// Markdown supports GitHub-style alerts such as "> [!NOTE]" and
// fenced admonitions such as "!!!note". Image syntax pointing at
// audio or video files, for example "![demo](demo.mp4)", renders as
// media controls.
//
// The command is a thin wrapper around package github.com/tmc/md2html.
//
//go:generate gocmddoc -o ../../README.md
package main
