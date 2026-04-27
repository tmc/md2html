// Package mdvet runs vet-style checks on Markdown files.
//
// The package mirrors the shape of go vet: a set of independent
// [Check] implementations, each producing [Diagnostic] values for a
// parsed Markdown document. [Run] applies a chosen set of checks to a
// list of files or directories.
//
// The initial check is [LinkCheck], which verifies that GitHub-style
// relative links in a document resolve to a path on disk. Links with
// a scheme (http, https, mailto, ...), pure fragment links, and links
// flagged as external are skipped.
package mdvet
