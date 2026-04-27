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
//
// Flags:
//
//	-c list   comma-separated list of checks to run (default: all)
//	-list     print the available checks and exit
//
// mdvet exits with status 1 when one or more diagnostics are reported,
// 2 on a usage or I/O error, and 0 otherwise.
package main
