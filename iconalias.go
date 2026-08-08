package md2html

import "html/template"

// iconGroups collects the names different icon sets give the same glyph.
//
// Documentation written for Mintlify names Font Awesome icons, while the
// sets that can be unpacked into an -icons directory are usually Lucide
// or Tabler. The three agree on most names ("rocket", "terminal",
// "download") and disagree on these, so the same Markdown renders with
// whichever set the author happens to have.
//
// Each group is written most-specific first, which is the order the
// names are tried in. A name belongs to a group only when the glyph
// means the same thing: where a set has no counterpart, it is left out,
// because no icon reads better than a wrong one. A name may appear in
// only one group, since a name in two would resolve differently
// depending on which was consulted first; [TestIconGroupsAreDisjoint]
// enforces that.
var iconGroups = [][]string{
	{"diagram-project", "workflow", "sitemap", "git-fork"},
	{"network-wired", "network", "share-2"},
	{"circle-question", "circle-help", "help-circle"},
	{"circle-exclamation", "circle-alert", "alert-circle"},
	{"triangle-exclamation", "triangle-alert", "alert-triangle"},
	{"circle-info", "info", "info-circle"},
	{"circle-check", "check-circle", "circle-check-big"},
	{"circle-play", "play-circle", "play"},
	{"code-compare", "git-compare", "git-compare-arrows"},
	{"shield-halved", "shield-half", "shield-check", "shield"},
	{"magnifying-glass", "search", "zoom-in"},
	{"screwdriver-wrench", "wrench", "tool", "settings-2"},
	{"file-lines", "file-text", "file"},
	{"rectangle-list", "list", "list-checks"},
	{"gauge-high", "gauge", "gauge-circle"},
	{"puzzle-piece", "puzzle"},
	{"robot", "bot"},
	{"square-check", "check-square", "check"},
	{"rectangle-terminal", "square-terminal", "terminal"},
	{"vial", "test-tube", "flask", "flask-conical", "beaker"},
	{"gear", "gears", "settings", "cog"},
	{"bolt", "zap"},
	{"box", "package"},
	{"house", "home"},
}

// iconAliases maps each name to the other names for the same glyph, in
// the order they are tried. It is derived from [iconGroups] so the two
// directions cannot drift: a page naming the Font Awesome spelling
// resolves against a Lucide directory and the other way round.
var iconAliases = buildIconAliases()

func buildIconAliases() map[string][]string {
	aliases := make(map[string][]string)
	for _, group := range iconGroups {
		for i, name := range group {
			others := make([]string, 0, len(group)-1)
			others = append(others, group[i+1:]...)
			others = append(others, group[:i]...)
			aliases[name] = others
		}
	}
	return aliases
}

// resolveIcon reports the markup for the icon a page asked for, trying
// the name itself and then the names known to draw the same glyph.
func resolveIcon(set map[string]template.HTML, name string) template.HTML {
	if svg, ok := set[name]; ok {
		return svg
	}
	for _, alias := range iconAliases[name] {
		if svg, ok := set[alias]; ok {
			return svg
		}
	}
	return ""
}
