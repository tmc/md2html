package md2html

// isDependencyDir reports whether a directory named name holds installed
// dependencies rather than the site's own pages. Packages ship READMEs
// and changelogs by the hundred, and no ignore file is needed to know
// they are not documentation of the tree being served. Everything that
// enumerates the tree skips them; a page inside one is still served
// when asked for by URL.
func isDependencyDir(name string) bool {
	return name == "node_modules"
}
