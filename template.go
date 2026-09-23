package md2html

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
)

func loadAllTemplates(site *preparedSite) (*template.Template, error) {
	if site == nil {
		site = &preparedSite{}
	}
	tmpl, err := template.New("root").Funcs(template.FuncMap{
		"default": func(def, val any) any {
			if val == nil {
				return def
			}
			if s, ok := val.(string); ok && s == "" {
				return def
			}
			return val
		},
		"loadJSON": func(filename string) any {
			// A template names its data relative to the same base as the
			// rest of the configuration, not to the process working
			// directory, which Run leaves alone.
			data, err := loadJSONFile(resolveAgainst(site.base, filename))
			if err != nil {
				slog.Default().Error("Error loading JSON", "file", filename, "error", err)
				return nil
			}
			return data
		},
		"replace": strings.ReplaceAll,
		// dict creates a map from key-value pairs for passing to templates
		"dict": func(values ...any) map[string]any {
			if len(values)%2 != 0 {
				return nil
			}
			dict := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					continue
				}
				dict[key] = values[i+1]
			}
			return dict
		},
		"navHref": func(currentFile, targetFile, htmlExt, indexFile string) string {
			return relativeRenderedLink(currentFile, targetFile, htmlExt, indexFile)
		},
		"navIcon": site.navIconType,
		"asset": func(assets map[string]string, name string) string {
			if assets != nil {
				if v := assets[name]; v != "" {
					return v
				}
			}
			return name
		},
		"jsonSpec": func() template.JS {
			return site.jsonSpecBundle
		},
	}).ParseFS(templates, "templates/*.html", "templates/*/*.html")

	if err != nil {
		slog.Default().Error("Error parsing embedded templates", "error", err)
		tmpl = template.New("root")
	}

	if site.config.TemplateDir != "" {
		for _, pattern := range []string{"*.html", "*/*.html"} {
			if t, err := tmpl.ParseGlob(filepath.Join(site.config.TemplateDir, pattern)); err == nil {
				tmpl = t
			}
		}
	}

	return tmpl, nil
}

// RenderOptions contains optional data passed to rendering and templates.
//
// Its exported fields are part of the template compatibility contract and
// follow semantic versioning.
type RenderOptions struct {
	// Nav is the navigation context for the current page.
	Nav *NavContext
	// SiteTitle is the configured site title.
	SiteTitle string
	// Data is the decoded value loaded from -data-json.
	Data any
	// FilePath is the source Markdown path relative to the rendered tree.
	FilePath string
	// Version is the currently rendered git version, when versioning is enabled.
	Version string
	// Versions is the list of available git-backed documentation versions.
	Versions []GitVersion
	// RawMDURL is the URL for the source Markdown file, when available.
	RawMDURL string
	// Description is the page description used for metadata and search.
	Description string
	// EditURL is the resolved edit link for the current page.
	EditURL string
	// LastUpdated is the page's last modification date, when known.
	LastUpdated string
	// Assets maps logical asset names to emitted, fingerprinted paths.
	Assets map[string]string
	// Accent and AccentDark are the site's brand color for light and dark
	// rendering, as CSS hex colors. Empty leaves the built-in accent.
	Accent     string
	AccentDark string
	// Repo is the "owner/name" of the documented source repository and
	// RepoURL its address. Empty renders no repository link.
	Repo    string
	RepoURL string
	// NavLinks are plain links shown in the navigation bar.
	NavLinks []SiteLink
	// Stars is the repository's star count, already formatted. Empty
	// shows the link without a count.
	Stars string
	// ShowStars reports that a count was asked for. A deployed page
	// refreshes the count in the browser, so the element has to be
	// rendered even when the build could not fetch one.
	ShowStars bool
}

func firstFrontmatterString(frontmatter map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := frontmatter[key]
		if !ok {
			continue
		}
		s, ok := value.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

func resolveMermaidThemes(frontmatter map[string]any) (theme, darkTheme string, auto bool) {
	theme = "default"
	darkTheme = "dark"
	auto = true

	userTheme := firstFrontmatterString(frontmatter,
		"mermaid_theme",
		"mermaid-theme",
		"mermaidTheme",
	)
	userDarkTheme := firstFrontmatterString(frontmatter,
		"mermaid_dark_theme",
		"mermaid-dark-theme",
		"mermaidDarkTheme",
		"mermaid_theme_dark",
		"mermaid-theme-dark",
		"mermaidThemeDark",
	)

	if userTheme != "" {
		if strings.EqualFold(userTheme, "auto") {
			auto = true
		} else {
			theme = userTheme
			darkTheme = userTheme
			auto = false
		}
	}

	if userDarkTheme != "" {
		darkTheme = userDarkTheme
		auto = true
	}

	return theme, darkTheme, auto
}

func renderTemplate(cfg Config, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]any) (string, error) {
	return renderTemplateWithOptions(cfg, htmlContent, title, customCSS, liveReload, frontmatter, RenderOptions{})
}

func renderTemplateWithOptions(cfg Config, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]any, opts RenderOptions) (string, error) {
	site, err := prepareSite(cfg, slog.Default())
	if err != nil {
		return "", err
	}
	return site.renderTemplate(htmlContent, title, customCSS, liveReload, frontmatter, opts)
}

func (s *preparedSite) renderTemplate(htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]any, opts RenderOptions) (string, error) {
	if s == nil {
		s = &preparedSite{}
	}
	tmpl, err := loadAllTemplates(s)
	if err != nil {
		return "", fmt.Errorf("load templates: %w", err)
	}

	name := "layout"
	if opts.Nav != nil && opts.Nav.HasNav {
		if tmpl.Lookup("docs-layout") != nil {
			name = "docs-layout"
		} else {
			slog.Default().Warn("SUMMARY.md navigation loaded but docs-layout template not found, falling back to layout")
		}
	}
	if tmpl.Lookup(name) == nil {
		for _, n := range []string{"docs.html", "page.html", "live-reload.html", "base"} {
			if tmpl.Lookup(n) != nil {
				name = n
				break
			}
		}
	}

	if tmpl.Lookup(name) == nil {
		return "", fmt.Errorf("no template found: expected layout template")
	}

	var buf bytes.Buffer
	mermaidTheme, mermaidDarkTheme, mermaidAutoTheme := resolveMermaidThemes(frontmatter)
	meta := pageMetadata(s.config, title, frontmatter, opts)
	var iconAttribution template.HTML
	if s.iconAttribution != "" {
		iconAttribution = template.HTML("<!-- " + s.iconAttribution + " -->")
	}

	data := templateData{
		Title:             title,
		Content:           template.HTML(htmlContent),
		CustomCSS:         template.CSS(customCSS),
		ChromaCSS:         template.CSS(generateChromaCSS()),
		Verbose:           s.config.Verbose,
		LiveReload:        liveReload,
		HTMLExt:           s.config.HTMLExt,
		Frontmatter:       frontmatter,
		Version:           opts.Version,
		Versions:          opts.Versions,
		Search:            s.config.Search,
		Nav:               opts.Nav,
		SiteTitle:         opts.SiteTitle,
		Data:              opts.Data,
		IndexFile:         s.config.Index,
		MermaidTheme:      mermaidTheme,
		MermaidDarkTheme:  mermaidDarkTheme,
		MermaidAutoTheme:  mermaidAutoTheme,
		FilePath:          opts.FilePath,
		AssetBase:         assetBase(opts.FilePath),
		RawMDURL:          opts.RawMDURL,
		HasMath:           pageHasMath(htmlContent),
		Description:       meta.Description,
		CanonicalURL:      meta.CanonicalURL,
		OpenGraphImage:    meta.OpenGraphImage,
		OpenGraphImageAlt: meta.OpenGraphImageAlt,
		OpenGraphType:     meta.OpenGraphType,
		LastUpdated:       meta.LastUpdated,
		EditURL:           opts.EditURL,
		Assets:            opts.Assets,
		Accent:            template.CSS(opts.Accent),
		AccentDark:        template.CSS(opts.AccentDark),
		Repo:              opts.Repo,
		RepoURL:           opts.RepoURL,
		NavLinks:          opts.NavLinks,
		Stars:             opts.Stars,
		ShowStars:         opts.ShowStars,
		IconAttribution:   iconAttribution,
	}

	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", name, err)
	}
	return buf.String(), nil
}

type templateData struct {
	Title            string
	Content          template.HTML
	CustomCSS        template.CSS
	ChromaCSS        template.CSS
	Verbose          bool
	LiveReload       bool
	HTMLExt          string
	Frontmatter      map[string]any
	Version          string
	Versions         []GitVersion
	Search           bool
	Nav              *NavContext
	SiteTitle        string
	Data             any
	IndexFile        string
	MermaidTheme     string
	MermaidDarkTheme string
	MermaidAutoTheme bool
	FilePath         string
	AssetBase        string
	RawMDURL         string
	// HasMath reports whether the page content contains TeX math
	// delimiters outside code regions, so templates can load MathJax
	// only where it is needed.
	HasMath      bool
	Description  string
	CanonicalURL string
	// OpenGraphImage is the absolute URL of the page's social card
	// image, OpenGraphImageAlt its description, and OpenGraphType the
	// og:type the page claims: "website" for the site root, "article"
	// for every other page.
	OpenGraphImage    string
	OpenGraphImageAlt string
	OpenGraphType     string
	LastUpdated       string
	EditURL           string
	Assets            map[string]string
	// Accent and AccentDark are validated CSS colors, empty unless the
	// navigation source named one.
	Accent     template.CSS
	AccentDark template.CSS
	// Repo and RepoURL name the source repository shown in the bar, and
	// Stars its formatted star count when one was fetched. ShowStars
	// reports that a count was asked for, which is what decides whether
	// the page carries the element the browser refreshes.
	Repo      string
	RepoURL   string
	Stars     string
	ShowStars bool
	// IconAttribution credits the built-in icon set in one place instead
	// of repeating its license comment in every inlined SVG.
	IconAttribution template.HTML
	// NavLinks are plain links shown in the navigation bar.
	NavLinks []SiteLink
}

type renderMetadata struct {
	Description       string
	CanonicalURL      string
	OpenGraphImage    string
	OpenGraphImageAlt string
	OpenGraphType     string
	LastUpdated       string
}

func pageMetadata(cfg Config, title string, frontmatter map[string]any, opts RenderOptions) renderMetadata {
	desc := firstFrontmatterString(frontmatter, "description")
	if desc == "" {
		desc = opts.Description
	}
	meta := renderMetadata{
		Description:       desc,
		OpenGraphImage:    firstFrontmatterString(frontmatter, "og_image", "image"),
		OpenGraphImageAlt: firstFrontmatterString(frontmatter, "og_image_alt", "image_alt"),
		OpenGraphType:     "article",
		LastUpdated:       opts.LastUpdated,
	}
	if opts.FilePath != "" {
		rendered := renderedPathForSource(opts.FilePath, cfg.HTMLExt, cfg.Index)
		page := canonicalPagePath(rendered, cfg.HTMLExt)
		if page == "" {
			meta.OpenGraphType = "website"
		}
		if cfg.SiteURL != "" {
			meta.CanonicalURL = joinSiteURL(cfg.SiteURL, page)
		}
	}
	// A card image has to be an absolute URL: the crawler fetches it
	// without a document to resolve against. The page URL is the base,
	// so a page-relative name in frontmatter means what it says, while
	// the site-wide default is written relative to the site root.
	if meta.OpenGraphImage != "" {
		meta.OpenGraphImage = absoluteURL(meta.CanonicalURL, meta.OpenGraphImage)
	} else if cfg.OGImage != "" {
		base := strings.TrimRight(strings.TrimSpace(cfg.SiteURL), "/")
		if base != "" {
			base += "/"
		}
		meta.OpenGraphImage = absoluteURL(base, cfg.OGImage)
		meta.OpenGraphImageAlt = ""
	}
	return meta
}

// absoluteURL resolves ref against base. A ref that is already absolute
// is returned unchanged, and so is one that cannot be resolved because
// no base URL was configured: half a URL is no more useful than a
// relative one, and dropping it would hide the mistake.
func absoluteURL(base, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	if u.IsAbs() || strings.HasPrefix(ref, "//") {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil || !b.IsAbs() {
		return ref
	}
	return b.ResolveReference(u).String()
}

func joinSiteURL(base, pagePath string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	pagePath = strings.TrimLeft(filepath.ToSlash(pagePath), "/")
	if base == "" {
		return base
	}
	return base + "/" + pagePath
}

func editURL(pattern, filePath string) string {
	if pattern == "" || filePath == "" {
		return ""
	}
	return strings.ReplaceAll(pattern, "{path}", filepath.ToSlash(filePath))
}
