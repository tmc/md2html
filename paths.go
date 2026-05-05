package md2html

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func sourceRoot(source string) (string, error) {
	if source == "" || source == "-" {
		return os.Getwd()
	}

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
