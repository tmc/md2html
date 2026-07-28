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
