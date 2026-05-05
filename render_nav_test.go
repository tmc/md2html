package md2html

import (
	"strings"
	"testing"
)

func TestRenderTemplateNavGroupsAreLabels(t *testing.T) {
	nav := &Navigation{
		Items: []*NavItem{
			{Title: "Getting Started", IsGroup: true},
			{Title: "Install", Path: "install.md", URL: "install.html"},
		},
		ByPath: map[string]*NavItem{"install.md": {Title: "Install", Path: "install.md", URL: "install.html"}},
		Flat:   []*NavItem{{Title: "Install", Path: "install.md", URL: "install.html"}},
	}
	opts := RenderOptions{
		Nav:       nav.ForPage("install.md"),
		SiteTitle: "Docs",
		FilePath:  "install.md",
	}

	got := renderTemplateWithOptions(Config{HTMLExt: "html"}, "<p>body</p>", "Install", "", false, nil, opts)
	if strings.Contains(got, `href="#section-`) {
		t.Fatalf("rendered orphan section anchor:\n%s", got)
	}
	if !strings.Contains(got, `<span class="top-nav-link">Getting Started</span>`) {
		t.Fatalf("rendered nav missing group label:\n%s", got)
	}
}
