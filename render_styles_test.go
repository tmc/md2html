package md2html

import (
	"strings"
	"testing"
)

func TestRenderTemplateStylesTaskListCheckboxes(t *testing.T) {
	got := mustRenderTemplateWithOptions(t, Config{}, "<ul><li><input checked disabled type=\"checkbox\"> Done</li></ul>", "Tasks", "", false, nil, RenderOptions{})
	for _, want := range []string{
		`li:has(> input[type="checkbox"][disabled]:first-child)`,
		`list-style-type: none;`,
		`li > input[type="checkbox"][disabled]:first-child`,
		`appearance: none;`,
		`border: 2px solid var(--fg-muted);`,
		`opacity: 1;`,
		`li > input[type="checkbox"][disabled]:first-child:checked`,
		`background: var(--accent);`,
		`clip-path: polygon`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered styles missing %q in:\n%s", want, got)
		}
	}
}

// TestRenderNoHardWraps checks that a paragraph wrapped in the source
// reflows rather than breaking at the author's wrap column. GFM treats a
// single newline as a space.
func TestRenderNoHardWraps(t *testing.T) {
	got, err := markdownToHTMLWithContext(Config{}, "one\ntwo\n\nthree  \nfour\n", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "<p>one<br />") {
		t.Errorf("single newline became a hard break:\n%s", got)
	}
	if !strings.Contains(got, "<p>one\ntwo</p>") {
		t.Errorf("paragraph did not reflow:\n%s", got)
	}
	// Two trailing spaces are an explicit break and must still render one.
	if !strings.Contains(got, "three<br />") {
		t.Errorf("explicit hard break was dropped:\n%s", got)
	}
}
