package md2html

import "strings"

// Agent skills are documented in a file named SKILL.md sitting alone in a
// directory named for the skill: skills/it2/SKILL.md, or
// plugins/it2/SKILL.md. The file describes the skill and its frontmatter
// declares the contract: "name", "description", and often "when_to_use"
// and "allowed-tools".
//
// Two of those conventions matter to a renderer that would otherwise treat
// SKILL.md as an ordinary page. It is the directory's landing file, so the
// skill answers to /skills/it2 rather than /skills/it2/SKILL; and it names
// itself in "name" rather than "title", so the alternative is to take the
// first heading, which is prose ("it2 - iTerm2 CLI Automation") where a
// label is wanted.

// isSkillFile reports whether base is the file name an agent skill uses.
func isSkillFile(base string) bool {
	return strings.EqualFold(base, "SKILL.md") || strings.EqualFold(base, "SKILL.markdown")
}

// skillName returns the name an agent skill declares for itself, or "" if
// the document is not a skill or declares none.
func skillName(base string, doc DocumentData) string {
	if !isSkillFile(base) {
		return ""
	}
	return firstFrontmatterString(doc.Frontmatter, "name")
}
