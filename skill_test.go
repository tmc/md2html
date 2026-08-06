package md2html

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAutoNavigationSkillFiles covers the layout an agent skill uses: a
// SKILL.md alone in a directory named for the skill. The file is the
// directory's landing page, so the directory becomes a group rather than
// gaining a "SKILL" child, and the group takes its name from the
// frontmatter rather than from the first heading.
func TestAutoNavigationSkillFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("skills/it2/SKILL.md", "---\nname: it2\ndescription: Control iTerm2 terminals.\n---\n\n# it2 - iTerm2 CLI Automation\n")
	write("skills/it2/reference.md", "# Reference\n")
	write("skills/nlm/SKILL.md", "---\nname: nlm\n---\n\n# nlm — NotebookLM CLI\n")

	nav, err := autoNavigationFromDir(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	// it2 has a page beside its SKILL.md, so it is a group named for the
	// skill whose first entry is the skill itself.
	var group *NavItem
	var leaves []*NavItem
	for _, item := range nav.Items {
		if item.IsGroup {
			group = item
		} else {
			leaves = append(leaves, item)
		}
	}
	if group == nil {
		t.Fatalf("no group for the skill directory: %+v", nav.Items)
	}
	if got := group.Title; got != "it2" {
		t.Errorf("group title = %q, want the frontmatter name", got)
	}
	if len(group.Children) != 2 {
		t.Fatalf("got %d children, want the skill and the page beside it", len(group.Children))
	}
	if got := group.Children[0]; got.Title != "it2" || got.Path != "skills/it2/SKILL.md" {
		t.Errorf("first child = %+v, want the skill page leading", got)
	}
	if got := group.Children[1].Title; got != "Reference" {
		t.Errorf("second child = %q, want Reference", got)
	}

	// nlm is a skill and nothing else, so it needs no group of one.
	if len(leaves) != 1 {
		t.Fatalf("got %d top-level pages, want the lone skill: %+v", len(leaves), leaves)
	}
	if got := leaves[0]; got.Title != "nlm" || got.Path != "skills/nlm/SKILL.md" {
		t.Errorf("top-level page = %+v, want the nlm skill", got)
	}
}

func TestAutoNavTitleSkillPrefersTitle(t *testing.T) {
	doc := DocumentData{
		Frontmatter: map[string]any{"title": "Explicit", "name": "it2"},
		Content:     "# Heading\n",
	}
	if got := autoNavTitle("SKILL.md", "skill", doc); got != "Explicit" {
		t.Errorf("got %q, want the explicit title to win over name", got)
	}
}

// TestAutoNavTitleNameOnlyForSkills guards the narrow reading of "name":
// it is a skill's own key, not a general title source, so an ordinary
// page that happens to carry one keeps its heading.
func TestAutoNavTitleNameOnlyForSkills(t *testing.T) {
	doc := DocumentData{
		Frontmatter: map[string]any{"name": "widget"},
		Content:     "# Configuring Widgets\n",
	}
	if got := autoNavTitle("guide.md", "guide", doc); got != "Configuring Widgets" {
		t.Errorf("got %q, want the heading", got)
	}
}

func TestLLMSTitleSkill(t *testing.T) {
	doc := DocumentData{
		Frontmatter: map[string]any{"name": "it2"},
		Content:     "# it2 - iTerm2 CLI Automation\n",
	}
	if got := llmsTitle("skills/it2/SKILL.md", doc); got != "it2" {
		t.Errorf("got %q, want the skill name", got)
	}
}
