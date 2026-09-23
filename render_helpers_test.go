package md2html

import "testing"

func mustRenderMarkdown(t *testing.T, cfg Config, markdown, filePath string) string {
	t.Helper()
	html, err := markdownToHTML(cfg, markdown, filePath)
	if err != nil {
		t.Fatal(err)
	}
	return html
}

func mustRenderTemplate(t *testing.T, cfg Config, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]any) string {
	t.Helper()
	html, err := renderTemplate(cfg, htmlContent, title, customCSS, liveReload, frontmatter)
	if err != nil {
		t.Fatal(err)
	}
	return html
}

func mustRenderTemplateWithOptions(t *testing.T, cfg Config, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]any, opts RenderOptions) string {
	t.Helper()
	html, err := renderTemplateWithOptions(cfg, htmlContent, title, customCSS, liveReload, frontmatter, opts)
	if err != nil {
		t.Fatal(err)
	}
	return html
}

func mustRenderSiteTemplateWithOptions(t *testing.T, site *preparedSite, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]any, opts RenderOptions) string {
	t.Helper()
	html, err := site.renderTemplate(htmlContent, title, customCSS, liveReload, frontmatter, opts)
	if err != nil {
		t.Fatal(err)
	}
	return html
}
