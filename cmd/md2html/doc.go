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
// Generate versioned static output, one directory per release tag plus
// the working tree as latest:
//
//	md2html -html _site -versions -version-pattern 'v*' -nav docs
//
// Each version is built from its own commit, checked out into a git
// worktree, so its pages, navigation, included code, and search index
// are those of the release. The output root holds versions.json, which
// every page's version switcher reads when it is viewed, a versions/
// page listing them, and redirects from the unversioned URLs to the
// default version. A versions.yaml file in the source directory, read
// from the working tree only, chooses which tags to publish and how to
// label them, and lists archived versions built by other tools, which
// are copied in verbatim:
//
//	since: v0.1.17     # publish no tag older than this
//	exclude: ["*-rc*"] # nor these
//	collapse: minor # list only the newest patch of each minor version
//	labels:
//	  latest: main
//	archive:
//	  - name: v0.1.16
//	    label: v0.1.16 and earlier
//	    path: archive/v0.1.16 # directory, or URL of a .tar.gz
//	banner:
//	  old: "You are reading docs for {version}. The latest is {latest}."
//
// When a page moves, git rename detection, or an aliases list in the
// moved page's frontmatter, lets the switcher follow it between
// versions, and the latest version keeps a redirect at the old path.
// With -version-cache, a version whose commit and chrome are unchanged
// is copied from the cache rather than rebuilt; -versions-only builds a
// subset; -version-alias stable=newest-tag publishes a copy of the
// newest release as stable/. With -http, the working tree is served at
// the root and each other version under /v/<name>/, from a worktree of
// its commit made when it is first requested.
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
