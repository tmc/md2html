// Command mdvet runs vet-style checks on Markdown files.
//
// Usage:
//
//	mdvet [flags] path...
//
// Each path may be a Markdown file or a directory; directories are
// walked recursively for .md, .markdown, and .mdx files.
//
// Available checks:
//
//	nav                SUMMARY.md lists every page that is not a draft
//	frontmatter        frontmatter is well formed and the page has a title
//	assets             local links, images, and media exist
//	links              relative link targets exist on disk
//	images             image src targets exist on disk
//	anchors            "#section" fragments resolve to a heading
//	duplicate-anchors  no two headings slug to the same id
//	reference-defs     [text][ref] has a matching definition (no orphans)
//	code-fence-lang    fenced code blocks declare a language
//	heading-skip       no heading-level jumps; at most one h1
//	case               link path matches on-disk casing
//	raw-html           no raw HTML in prose, which renders to nothing
//	components         component tags are known, complete, and closed
//	includes           <Code> tags name files md2html can include
//	icons              icon names exist in the selected icon set
//	gitignored         link targets are carried by the repository
//
// Flags:
//
//	-c list      comma-separated list of checks to run (default: all)
//	-list        print the available checks and exit
//	-components dir
//	             directory of component definitions, as passed to md2html
//	-base path   URL path prefix the tree is served under (e.g. /docs)
//	-include-root dir
//	             directory files included with <Code src> must be
//	             inside, as passed to md2html
//	-versions    also vet the directory as of each published git tag
//	-version-pattern glob
//	             git tags to vet with -versions, as passed to md2html
//	-version-config file
//	             version policy file, as passed to md2html
//
// Docs written for a hosted site link to each other by rendered URL:
// "/docs/churl#exit-status" rather than "churl.md#exit-status". Such a
// path names no file on disk, so without -base neither the target nor
// its anchor can be checked. With it, mdvet maps the URL back to the
// source file that renders it. A rooted link inside the prefix that
// resolves to no page is then reported as broken.
//
// With -versions, mdvet vets a versioned site as md2html -versions
// publishes it: the directory as it is in the working tree, then as it
// was at each published tag, checked out into a git worktree. A tag's
// diagnostics name files as tag:path, as git does. Two checks then run
// across versions:
//
//	version-aliases  an aliases entry in frontmatter names a page that
//	                 exists in some version
//	version-removed  a page in the newest release still exists, or was
//	                 moved by a rename git detects or an alias
//
// mdvet exits with status 1 when one or more diagnostics are reported,
// 2 on a usage or I/O error, and 0 otherwise.
package main
