// Package docversions finds the versions of a documentation tree kept in
// a git repository and describes them to the pages that switch between
// them.
//
// A version is a git ref: a tag matching a pattern, a branch, or the
// working tree, published as "latest". Everything a version's pages are
// built from is read at that ref; nothing is mixed across refs. A
// [Config], read from the development tree, says which tags to publish
// and how to label them, and the [Manifest] built from them is what
// every page's version switcher reads.
package docversions
