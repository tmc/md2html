package docversions

import (
	"path"
	"strings"

	"github.com/tmc/md2html/internal/markdownfile"
)

// Aliases returns the aliases a page's frontmatter lists: the paths
// the page was published at before it moved, as in
//
//	aliases: [modules/agents/agents]
//
// The value may be one string or a list of them.
func Aliases(frontmatter map[string]any) []string {
	switch v := frontmatter["aliases"].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// AliasSource returns the source page an alias names: a path relative
// to the source root, with or without its extension.
func AliasSource(alias string) string {
	alias = path.Clean("/" + strings.TrimSpace(alias))[1:]
	if !markdownfile.Is(alias) {
		alias += ".md"
	}
	return alias
}
