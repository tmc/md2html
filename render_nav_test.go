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

	got := mustRenderTemplateWithOptions(t, Config{HTMLExt: "html"}, "<p>body</p>", "Install", "", false, nil, opts)
	if strings.Contains(got, `href="#section-`) {
		t.Fatalf("rendered orphan section anchor:\n%s", got)
	}
	if !strings.Contains(got, `<li class="nav-group">Getting Started</li>`) {
		t.Fatalf("rendered nav missing group label:\n%s", got)
	}
	// The bar above the sidebar names the site, not every group.
	if !strings.Contains(got, `<span class="top-nav-title">Docs</span>`) {
		t.Fatalf("rendered nav missing site title in the top bar:\n%s", got)
	}
}

// TestRenderTemplateNavGroupChildren checks that the pages inside a group
// are rendered. A group whose children were dropped left the sidebar with
// nothing but headings.
func TestRenderTemplateNavGroupChildren(t *testing.T) {
	install := &NavItem{Title: "Install", Path: "install.md", URL: "install.html", Level: 1}
	nav := &Navigation{
		Items: []*NavItem{{Title: "Getting Started", IsGroup: true, Children: []*NavItem{install}}},
	}
	nav.buildIndexes()
	opts := RenderOptions{
		Nav:       nav.ForPage("install.md"),
		SiteTitle: "Docs",
		FilePath:  "install.md",
	}

	got := mustRenderTemplateWithOptions(t, Config{HTMLExt: "html"}, "<p>body</p>", "Install", "", false, nil, opts)
	if !strings.Contains(got, `<li class="nav-group">Getting Started</li>`) {
		t.Fatalf("rendered nav missing group label:\n%s", got)
	}
	if !strings.Contains(got, ">\n        Install\n      </a>") && !strings.Contains(got, ">Install<") {
		t.Fatalf("rendered nav missing the page inside the group:\n%s", got)
	}
}

// TestRenderTemplateNavIcons checks that a nav icon reaches the page as
// an inert marker. The name is carried in a data attribute so a
// stylesheet can attach the glyph; nothing here depends on an icon set
// being present, and an item without an icon gets no marker at all.
func TestRenderTemplateNavIcons(t *testing.T) {
	install := &NavItem{Title: "Install", Path: "install.md", URL: "install.html", Icon: "rocket"}
	plain := &NavItem{Title: "Plain", Path: "plain.md", URL: "plain.html"}
	nav := &Navigation{Items: []*NavItem{install, plain}}
	nav.buildIndexes()
	opts := RenderOptions{
		Nav:       nav.ForPage("install.md"),
		SiteTitle: "Docs",
		FilePath:  "install.md",
	}

	got := mustRenderTemplateWithOptions(t, Config{HTMLExt: "html"}, "<p>body</p>", "Install", "", false, nil, opts)
	if !strings.Contains(got, `<span class="nav-icon" data-icon="rocket" aria-hidden="true">`) {
		t.Fatalf("rendered nav missing icon marker:\n%s", got)
	}
	if strings.Count(got, `class="nav-icon"`) != 1 {
		t.Fatalf("icon marker rendered for an item without an icon:\n%s", got)
	}
}

// TestRenderTemplateNavIconSVG checks that a configured icon set reaches
// the page. Without a set the marker is empty, so the glyph arriving is
// what distinguishes a working icon directory from a missing one.
func TestRenderTemplateNavIconSVG(t *testing.T) {
	dir := writeIconSet(t, map[string]string{
		"rocket": `<svg viewBox="0 0 24 24"><path d="M12 15v5"/></svg>`,
	})
	cfg, err := prepareIcons(Config{HTMLExt: "html", Icons: dir})
	if err != nil {
		t.Fatalf("prepareIcons() error = %v", err)
	}

	install := &NavItem{Title: "Install", Path: "install.md", URL: "install.html", Icon: "rocket"}
	absent := &NavItem{Title: "Absent", Path: "absent.md", URL: "absent.html", Icon: "no-such-icon"}
	nav := &Navigation{Items: []*NavItem{install, absent}}
	nav.buildIndexes()
	opts := RenderOptions{Nav: nav.ForPage("install.md"), SiteTitle: "Docs", FilePath: "install.md"}

	got := mustRenderTemplateWithOptions(t, cfg, "<p>body</p>", "Install", "", false, nil, opts)
	if !strings.Contains(got, `<path d="M12 15v5"/>`) {
		t.Fatalf("rendered nav did not inline the icon:\n%s", got)
	}
	// The SVG is markup, not text: an escaped angle bracket would mean
	// the glyph is displayed as source instead of drawn.
	if strings.Contains(got, "&lt;svg") {
		t.Fatalf("icon was escaped instead of inlined:\n%s", got)
	}
	// A name the set does not have leaves an empty marker rather than
	// failing the render.
	if !strings.Contains(got, `data-icon="no-such-icon" aria-hidden="true"></span>`) {
		t.Fatalf("unknown icon name did not render an empty marker:\n%s", got)
	}
}
