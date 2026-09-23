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
	// The bar above the sidebar names the site, not every group, and
	// the name links home.
	if !strings.Contains(got, `class="top-nav-title" href="./">Docs</a>`) {
		t.Fatalf("rendered nav missing site title in the top bar:\n%s", got)
	}
}

// TestRenderTemplateNavGroupChildren checks that the pages inside a group
// are rendered, so the sidebar holds pages and not only group headings.
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
	absent := &NavItem{Title: "Absent", Path: "absent.md", URL: "absent.html", Icon: "clock"}
	nav := &Navigation{Items: []*NavItem{install, absent}}
	nav.buildIndexes()
	opts := RenderOptions{Nav: nav.ForPage("install.md"), SiteTitle: "Docs", FilePath: "install.md"}

	got := mustRenderSiteTemplateWithOptions(t, cfg, "<p>body</p>", "Install", "", false, nil, opts)
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
	if !strings.Contains(got, `data-icon="clock" aria-hidden="true"></span>`) {
		t.Fatalf("custom icon directory fell back to the built-in set:\n%s", got)
	}
}

func TestRenderTemplateBuiltinIconStyleAndAttribution(t *testing.T) {
	cfg, err := prepareBuiltinIcons(Config{HTMLExt: "html"}, "fontawesome")
	if err != nil {
		t.Fatal(err)
	}
	item := &NavItem{Title: "History", Path: "history.md", URL: "history.html", Icon: "clock", IconType: "regular"}
	nav := &Navigation{Items: []*NavItem{item}}
	nav.buildIndexes()
	opts := RenderOptions{Nav: nav.ForPage("history.md"), SiteTitle: "Docs", FilePath: "history.md"}
	got := mustRenderSiteTemplateWithOptions(t, cfg, "<p>body</p>", "History", "", false, nil, opts)
	want := string(cfg.navIconType("clock", "regular"))
	if want == "" || !strings.Contains(got, want) {
		t.Fatalf("rendered navigation did not use the requested regular icon")
	}
	if strings.Count(got, cfg.iconAttribution) != 1 {
		t.Errorf("built-in attribution count = %d, want 1", strings.Count(got, cfg.iconAttribution))
	}
}

// TestRenderTemplateRepoLink checks that a repository from docs.json is
// shown in the bar, and that a site without one shows nothing rather
// than an empty link.
func TestRenderTemplateRepoLink(t *testing.T) {
	nav := &Navigation{Items: []*NavItem{{Title: "Install", Path: "install.md", URL: "install.html"}}}
	nav.buildIndexes()
	base := RenderOptions{Nav: nav.ForPage("install.md"), SiteTitle: "Docs", FilePath: "install.md"}

	opts := base
	opts.Repo, opts.RepoURL = "tmc/cdp", "https://github.com/tmc/cdp"
	got := mustRenderTemplateWithOptions(t, Config{HTMLExt: "html"}, "<p>body</p>", "Install", "", false, nil, opts)
	for _, want := range []string{
		`class="repo-link" href="https://github.com/tmc/cdp"`,
		`<span class="repo-link-name">tmc/cdp</span>`,
		`rel="noreferrer noopener"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered bar missing %q:\n%s", want, got)
		}
	}

	got = mustRenderTemplateWithOptions(t, Config{HTMLExt: "html"}, "<p>body</p>", "Install", "", false, nil, base)
	// The stylesheet always defines .repo-link, so look for the anchor.
	if strings.Contains(got, `class="repo-link" href=`) {
		t.Errorf("rendered a repository link for a site without one:\n%s", got)
	}
}

// TestRenderTemplateStarsRefresh checks what a deployed page carries: a
// count the browser can update, and the script that updates it. The
// build-time count is only correct as of the build, so a static site
// without this shows a number that is stale from the day it ships.
func TestRenderTemplateStarsRefresh(t *testing.T) {
	nav := &Navigation{Items: []*NavItem{{Title: "Install", Path: "install.md", URL: "install.html"}}}
	nav.buildIndexes()
	base := RenderOptions{Nav: nav.ForPage("install.md"), SiteTitle: "Docs", FilePath: "install.md"}
	base.Repo, base.RepoURL = "tmc/cdp", "https://github.com/tmc/cdp"

	render := func(opts RenderOptions) string {
		return mustRenderTemplateWithOptions(t, Config{HTMLExt: "html"}, "<p>body</p>", "Install", "", false, nil, opts)
	}

	opts := base
	opts.Stars, opts.ShowStars = "4", true
	got := render(opts)
	for _, want := range []string{
		`<span class="repo-link-stars" data-repo="tmc/cdp">`,
		`<span class="repo-link-count">4</span>`,
		"api.github.com/repos/",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered page missing %q:\n%s", want, got)
		}
	}

	// A build with no network still has to leave the element behind, or
	// the browser has nothing to fill in.
	opts = base
	opts.ShowStars = true
	got = render(opts)
	if !strings.Contains(got, `<span class="repo-link-count"></span>`) {
		t.Errorf("no count element after a failed fetch:\n%s", got)
	}

	// Without -github-stars nothing is rendered and nothing is fetched:
	// the page still needs no network.
	got = render(base)
	if strings.Contains(got, `<span class="repo-link-count">`) || strings.Contains(got, "api.github.com") {
		t.Errorf("rendered star markup without -github-stars:\n%s", got)
	}
}
