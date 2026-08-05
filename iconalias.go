package md2html

import "html/template"

// iconAliases maps an icon name to the other names that draw the same
// glyph, tried in order when the icon directory has no file under the
// name a page asked for.
//
// Documentation written for Mintlify names Font Awesome icons, while the
// icon sets that can be unpacked into a directory and shipped with a
// site are usually Lucide or Tabler. The three agree on most names
// ("rocket", "terminal", "download"); this table covers the ones where
// they disagree, so the same Markdown renders with whichever set the
// author happens to have.
//
// A name belongs here only when the glyphs mean the same thing. Where no
// set has a counterpart, the name is left out: an entry has to be right
// for every set it might resolve against, and no icon reads better than
// a wrong one.
var iconAliases = map[string][]string{
	// Font Awesome name first, then the sets that spell it differently.
	"circle-question":      {"circle-help", "help-circle", "help"},
	"circle-exclamation":   {"circle-alert", "alert-circle"},
	"triangle-exclamation": {"triangle-alert", "alert-triangle"},
	"circle-info":          {"info", "info-circle"},
	"circle-check":         {"check-circle", "circle-check-big"},
	"circle-play":          {"play-circle", "play"},
	"code-compare":         {"git-compare", "git-compare-arrows"},
	"diagram-project":      {"workflow", "sitemap", "git-fork"},
	"network-wired":        {"network", "share-2"},
	"shield-halved":        {"shield-half", "shield-check", "shield"},
	"magnifying-glass":     {"search", "zoom-in"},
	"screwdriver-wrench":   {"wrench", "tool", "settings-2"},
	"file-lines":           {"file-text", "file"},
	"gauge-high":           {"gauge", "gauge-circle"},
	"square-check":         {"check-square", "check"},
	"rectangle-terminal":   {"square-terminal", "terminal"},
	"flask":                {"flask-conical", "beaker"},
	"vial":                 {"test-tube", "flask-conical", "beaker"},
	"gear":                 {"settings", "cog"},
	"gears":                {"settings", "cog"},
	"bolt":                 {"zap"},
	"box":                  {"package"},
	"house":                {"home"},

	// The reverse direction, so a page written against Lucide still
	// resolves against a Font Awesome directory.
	"circle-help":    {"circle-question"},
	"circle-alert":   {"circle-exclamation"},
	"triangle-alert": {"triangle-exclamation"},
	"flask-conical":  {"flask", "vial"},
	"test-tube":      {"vial", "flask"},
	"git-compare":    {"code-compare"},
	"settings":       {"gear", "cog"},
	"zap":            {"bolt"},
	"home":           {"house"},
	"search":         {"magnifying-glass"},
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
