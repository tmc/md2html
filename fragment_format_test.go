package md2html

import (
	"strings"
	"testing"
)

func TestRenderFragmentInvalidFormatUsesDefault(t *testing.T) {
	fragment, err := RenderFragment("[Orders](/tables/orders.md)", "docs/page.md", FragmentOptions{
		Format:  "unknown",
		HTMLExt: "html",
	})
	if err != nil {
		t.Fatalf("RenderFragment() error = %v", err)
	}
	if !strings.Contains(string(fragment.HTML), `href="/tables/orders.md"`) {
		t.Fatalf("RenderFragment() HTML = %q, want ordinary root link", fragment.HTML)
	}
}
