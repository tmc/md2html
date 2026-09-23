// Command mdvet runs vet-style checks on Markdown files.
//
// Usage:
//
//	mdvet [flags] path...
//
// Each path may be a Markdown file or a directory; directories are
// walked recursively for .md and .markdown files.
//
// Available checks:
//
//	links              relative link targets exist on disk
//	images             image src targets exist on disk
//	anchors            "#section" fragments resolve to a heading
//	duplicate-anchors  no two headings slug to the same id
//	reference-defs     [text][ref] has a matching definition (no orphans)
//	code-fence-lang    fenced code blocks declare a language
//	heading-skip       no heading-level jumps; at most one h1
//	case               link path matches on-disk casing
//	raw-html           no raw HTML in prose, which renders to nothing
//	gitignored         link targets are carried by the repository
//
// Flags:
//
//	-c list      comma-separated list of checks to run (default: all)
//	-list        print the available checks and exit
//	-base path   URL path prefix the tree is served under (e.g. /docs)
//
// Docs written for a hosted site link to each other by rendered URL:
// "/docs/churl#exit-status" rather than "churl.md#exit-status". Such a
// path names no file on disk, so without -base neither the target nor
// its anchor can be checked. With it, mdvet maps the URL back to the
// source file that renders it. A rooted link inside the prefix that
// resolves to no page is then reported as broken.
//
// mdvet exits with status 1 when one or more diagnostics are reported,
// 2 on a usage or I/O error, and 0 otherwise.
package main
