package md2html

import (
	"strings"
	"testing"
)

func TestRenderTemplateMetadata(t *testing.T) {
	cfg := Config{
		SiteURL: "https://example.com/docs/",
		HTMLExt: "html",
	}
	frontmatter := map[string]any{
		"description": "A useful page summary.",
		"og_image":    "https://example.com/og.png",
	}
	opts := RenderOptions{FilePath: "guide/intro.md"}

	got := mustRenderTemplateWithOptions(t, cfg, "<p>body</p>", "Intro", "", false, frontmatter, opts)
	for _, want := range []string{
		`<meta name="description" content="A useful page summary.">`,
		`<link rel="canonical" href="https://example.com/docs/guide/intro.html">`,
		`<meta property="og:title" content="Intro">`,
		`<meta property="og:description" content="A useful page summary.">`,
		`<meta property="og:image" content="https://example.com/og.png">`,
		`<meta property="og:url" content="https://example.com/docs/guide/intro.html">`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered metadata missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderTemplateMetadataIndexCanonical(t *testing.T) {
	cfg := Config{
		SiteURL: "https://example.com",
		HTMLExt: "html",
		Index:   "index.md",
	}

	got := mustRenderTemplateWithOptions(t, cfg, "<p>body</p>", "Home", "", false, nil, RenderOptions{FilePath: "index.md"})
	if !strings.Contains(got, `<link rel="canonical" href="https://example.com/">`) {
		t.Fatalf("root index canonical not collapsed to site root in:\n%s", got)
	}

	got = mustRenderTemplateWithOptions(t, cfg, "<p>body</p>", "Posts", "", false, nil, RenderOptions{FilePath: "posts/index.md"})
	if !strings.Contains(got, `<link rel="canonical" href="https://example.com/posts/">`) {
		t.Fatalf("nested index canonical not collapsed to directory in:\n%s", got)
	}
}

func TestRenderTemplateMetadataFallbackAndAbsence(t *testing.T) {
	cfg := Config{HTMLExt: "html"}
	opts := RenderOptions{
		FilePath:    "guide/intro.md",
		Description: "First paragraph summary.",
	}

	got := mustRenderTemplateWithOptions(t, cfg, "<p>body</p>", "Intro", "", false, nil, opts)
	if !strings.Contains(got, `<meta name="description" content="First paragraph summary.">`) {
		t.Fatalf("rendered metadata missing description fallback:\n%s", got)
	}
	for _, not := range []string{
		`rel="canonical"`,
		`property="og:image"`,
		`property="og:url"`,
		`content=""`,
	} {
		if strings.Contains(got, not) {
			t.Fatalf("rendered metadata unexpectedly contains %q in:\n%s", not, got)
		}
	}
}

func TestRenderTemplateFooterMetadata(t *testing.T) {
	opts := RenderOptions{
		FilePath:    "guide/intro.md",
		EditURL:     "https://example.com/edit/guide/intro.md",
		LastUpdated: "2026-05-05",
	}

	got := mustRenderTemplateWithOptions(t, Config{}, "<p>body</p>", "Intro", "", false, nil, opts)
	for _, want := range []string{
		`<div class="last-updated">Last updated: 2026-05-05</div>`,
		`<a class="edit-link" href="https://example.com/edit/guide/intro.md" target="_blank" rel="noopener">Edit this page</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered footer missing %q in:\n%s", want, got)
		}
	}
}
