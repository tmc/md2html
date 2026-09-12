package iconsets

// aliasGroups lists names used by different libraries for the same glyph.
var aliasGroups = [][]string{
	{"diagram-project", "arrows-turn-to-dots", "workflow", "sitemap", "git-fork"},
	{"network-wired", "network", "share-2"},
	{"signal-stream", "tower-broadcast", "broadcast-tower", "radio-tower", "broadcast"},
	{"library", "books", "book-bookmark", "book"},
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
	{"table-cells", "table-2", "table"},
	{"gauge-high", "gauge", "gauge-circle"},
	{"puzzle-piece", "puzzle"},
	{"robot", "bot"},
	{"square-check", "check-square", "check"},
	{"rectangle-terminal", "square-terminal", "terminal"},
	{"vial", "test-tube", "flask", "flask-conical", "beaker"},
	{"gear", "gears", "settings", "cog"},
	{"bolt", "zap"},
	{"cube", "box", "package"},
	{"boxes-stacked", "boxes"},
	{"bullseye", "target"},
	{"burst", "badge"},
	{"right-left", "arrow-right-left"},
	{"ruler-combined", "ruler"},
	{"house", "home"},
}

// AliasGroups returns the icon-name groups. The returned groups belong to the
// caller, so a consumer cannot change alias resolution for another caller.
func AliasGroups() [][]string {
	groups := make([][]string, len(aliasGroups))
	for i, group := range aliasGroups {
		groups[i] = append([]string(nil), group...)
	}
	return groups
}

// Aliases returns the other names in name's semantic group.
func Aliases(name string) []string {
	for _, group := range aliasGroups {
		for i, candidate := range group {
			if candidate == name {
				aliases := append([]string(nil), group[i+1:]...)
				return append(aliases, group[:i]...)
			}
		}
	}
	return nil
}
