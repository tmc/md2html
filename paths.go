package md2html

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// resolveBase returns the absolute directory that a Config's relative
// filesystem paths resolve against: the process working directory, or
// Config.Chdir resolved against it.
//
// -C names a base rather than changing the process working directory,
// so an embedding caller keeps its own directory and two configurations
// can be used at the same time. The directory is checked here, even
// when every named path is absolute, so a mistyped -C still says so.
func resolveBase(chdir string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	if chdir == "" {
		return wd, nil
	}
	base := chdir
	if !filepath.IsAbs(base) {
		base = filepath.Join(wd, base)
	}
	base = filepath.Clean(base)
	info, err := os.Stat(base)
	if err != nil {
		return "", fmt.Errorf("chdir: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("chdir: %s is not a directory", base)
	}
	return base, nil
}

// resolveAgainst resolves a filesystem path against base. An empty path
// keeps its meaning — "not configured" — and an absolute one is already
// resolved. An empty base leaves the path relative to the process
// working directory, which is what a zero-value site wants.
func resolveAgainst(base, name string) string {
	if name == "" || base == "" || filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(base, name)
}

// resolveConfigPaths returns cfg with every filesystem path resolved
// against base. URL paths — Base, SiteURL, EditURL — and document names
// relative to the rendered tree — Index — are not filesystem paths and
// are left alone.
func resolveConfigPaths(cfg Config, base string) Config {
	if cfg.Source != "-" {
		cfg.Source = resolveAgainst(base, cfg.Source)
	}
	cfg.HTML = resolveAgainst(base, cfg.HTML)
	cfg.CSS = resolveAgainst(base, cfg.CSS)
	cfg.TemplateDir = resolveAgainst(base, cfg.TemplateDir)
	cfg.DataJSON = resolveAgainst(base, cfg.DataJSON)
	cfg.JSONSpec = resolveAgainst(base, cfg.JSONSpec)
	cfg.Components = resolveAgainst(base, cfg.Components)
	cfg.Icons = resolveAgainst(base, cfg.Icons)
	return cfg
}

// sourceRoot returns the directory the served or rendered tree is rooted
// at. A source that names nothing, or stdin, has no directory of its
// own and is rooted at base.
func sourceRoot(base, source string) (string, error) {
	if source == "" || source == "-" {
		if base != "" {
			return base, nil
		}
		return os.Getwd()
	}
	source = resolveAgainst(base, source)

	info, err := os.Stat(source)
	if err == nil {
		if info.IsDir() {
			return filepath.Abs(source)
		}
		return filepath.Abs(filepath.Dir(source))
	}

	if filepath.Ext(source) != "" {
		return filepath.Abs(filepath.Dir(source))
	}
	return filepath.Abs(source)
}

func secureJoin(root, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root")
	}

	full := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return full, nil
}

func secureURLPath(name string) (string, error) {
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	clean := path.Clean(name)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes root")
	}
	return clean, nil
}

func normalizeHTMLExt(htmlExt string) string {
	if htmlExt == "" {
		return ""
	}
	if strings.HasPrefix(htmlExt, ".") {
		return htmlExt
	}
	return "." + htmlExt
}

func normalizeSourcePath(name string) string {
	if name == "" || name == "." {
		return ""
	}
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "./")
	name = path.Clean(name)
	if name == "." {
		return ""
	}
	return name
}

func renderedPathForSource(sourcePath, htmlExt, indexFile string) string {
	sourcePath = normalizeSourcePath(sourcePath)
	indexFile = normalizeSourcePath(indexFile)
	htmlExt = normalizeHTMLExt(htmlExt)

	if indexFile != "" && sourcePath == indexFile {
		if htmlExt != "" {
			return "index" + htmlExt
		}
		return "."
	}

	if sourcePath == "" {
		return ""
	}

	base := strings.TrimSuffix(sourcePath, path.Ext(sourcePath))
	if htmlExt != "" {
		return base + htmlExt
	}
	return base
}

// canonicalPagePath maps a rendered page path to the path used in its
// canonical URL, collapsing index pages into their directory:
// "index.html" becomes "" and "posts/index.html" becomes "posts/".
func canonicalPagePath(rendered, htmlExt string) string {
	htmlExt = normalizeHTMLExt(htmlExt)
	if htmlExt == "" {
		return rendered
	}
	rendered = filepath.ToSlash(rendered)
	if rendered == "index"+htmlExt {
		return ""
	}
	if dir, ok := strings.CutSuffix(rendered, "/index"+htmlExt); ok {
		return dir + "/"
	}
	return rendered
}

func relativeRenderedLink(currentSourcePath, targetSourcePath, htmlExt, indexFile string) string {
	target := renderedPathForSource(targetSourcePath, htmlExt, indexFile)
	if target == "" {
		return ""
	}

	current := renderedPathForSource(currentSourcePath, htmlExt, indexFile)
	if current == "" {
		return target
	}

	currentDir := path.Dir(current)
	if current == "." || currentDir == "" {
		currentDir = "."
	}

	rel, err := filepath.Rel(filepath.FromSlash(currentDir), filepath.FromSlash(target))
	if err != nil {
		return target
	}
	rel = filepath.ToSlash(rel)
	if rel == "" {
		return "."
	}
	return rel
}

// assetBase returns a relative URL prefix from the page rendered for
// currentSourcePath up to the output root, suitable for prefixing static
// asset paths like "js/search.js". It always ends with "/".
//
// Examples:
//
//	assetBase("README.md")        -> "./"
//	assetBase("docs/guide.md")    -> "../"
//	assetBase("a/b/c/page.md")    -> "../../../"
func assetBase(currentSourcePath string) string {
	clean := strings.TrimPrefix(normalizeSourcePath(currentSourcePath), "/")
	dir := path.Dir(clean)
	if dir == "." || dir == "" || dir == "/" {
		return "./"
	}
	return strings.Repeat("../", strings.Count(dir, "/")+1)
}

func splitLinkSuffix(raw string) (base, suffix string) {
	for i, r := range raw {
		if r == '#' || r == '?' {
			return raw[:i], raw[i:]
		}
	}
	return raw, ""
}

func rewriteLocalMarkdownReference(currentSourcePath, href, htmlExt, indexFile string) (string, bool) {
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "/") ||
		strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:") || strings.HasPrefix(href, "javascript:") ||
		strings.HasPrefix(href, "data:") {
		return href, false
	}

	base, suffix := splitLinkSuffix(href)
	ext := strings.ToLower(path.Ext(base))
	if ext != ".md" && ext != ".markdown" {
		return href, false
	}

	currentDir := path.Dir(normalizeSourcePath(currentSourcePath))
	if currentDir == "" {
		currentDir = "."
	}

	targetSourcePath := path.Clean(path.Join(currentDir, base))
	return relativeRenderedLink(currentSourcePath, targetSourcePath, htmlExt, indexFile) + suffix, true
}

// rewriteMarkdownReference rewrites local Markdown links for format. Ordinary
// Markdown keeps root-relative links unchanged. OKF uses root-relative links
// to identify concepts within the bundle, so its profile rewrites those links
// relative to the rendered page.
func rewriteMarkdownReference(format, currentSourcePath, href, htmlExt, indexFile string) (string, bool) {
	if normalizedFormat(format) != "okf" || !strings.HasPrefix(href, "/") {
		return rewriteLocalMarkdownReference(currentSourcePath, href, htmlExt, indexFile)
	}

	base, suffix := splitLinkSuffix(href)
	ext := strings.ToLower(path.Ext(base))
	if ext != ".md" && ext != ".markdown" {
		return href, false
	}
	targetSourcePath := strings.TrimPrefix(path.Clean(base), "/")
	return relativeRenderedLink(currentSourcePath, targetSourcePath, htmlExt, indexFile) + suffix, true
}

func validateFormat(format string) error {
	switch normalizedFormat(format) {
	case "", "okf":
		return nil
	default:
		return fmt.Errorf("invalid format %q (want okf)", format)
	}
}

func normalizedFormat(format string) string {
	return strings.ToLower(strings.TrimSpace(format))
}
