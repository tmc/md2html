package md2html

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SearchDocument represents a document in the search index.
type SearchDocument struct {
	Title    string `json:"title"`
	Text     string `json:"text"`
	Category string `json:"category"`
	URL      string `json:"url"`
	Blurb    string `json:"blurb"`
	Type     string `json:"type"`
}

// buildSearchIndex walks sourceDir and returns a search document for every
// markdown file found, ready to be marshaled and consumed by the client-side
// search.
func buildSearchIndex(sourceDir string, cfg Config) ([]SearchDocument, error) {
	var documents []SearchDocument

	ignore, err := loadIgnoreSet(sourceDir)
	if err != nil {
		return nil, err
	}
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if ignore.excludes(path, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", path, err)
		}

		doc, err := parseFrontmatter(string(content))
		if err != nil {
			doc = DocumentData{
				Content:     string(content),
				Frontmatter: make(map[string]any),
			}
		}

		title := ""
		if t, ok := doc.Frontmatter["title"].(string); ok {
			title = t
		} else {
			title = strings.TrimSuffix(filepath.Base(path), ".md")
		}

		blurb := ""
		if b, ok := doc.Frontmatter["description"].(string); ok {
			blurb = b
		} else if b, ok := doc.Frontmatter["blurb"].(string); ok {
			blurb = b
		} else {
			blurb = extractBlurb(doc.Content)
		}

		relPath, _ := filepath.Rel(sourceDir, path)
		category := filepath.Dir(relPath)
		if category == "." {
			category = ""
		}

		url := convertPathToURL(relPath, cfg.HTMLExt)
		plainText := extractPlainText(doc.Content)

		docType := "documentation"
		if t, ok := doc.Frontmatter["type"].(string); ok {
			docType = t
		}

		documents = append(documents, SearchDocument{
			Title:    title,
			Text:     plainText,
			Category: category,
			URL:      url,
			Blurb:    truncate(blurb, 200),
			Type:     docType,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	return documents, nil
}

// renderSearchIndexJS serializes documents into a JavaScript file that
// assigns window.MD2HTML_SEARCH_INDEX. Using a global variable instead of a
// fetched JSON file keeps search working when the rendered site is opened
// directly via file:// (the offline-by-default contract).
func renderSearchIndexJS(documents []SearchDocument) ([]byte, error) {
	data, err := json.Marshal(documents)
	if err != nil {
		return nil, fmt.Errorf("error marshaling search index: %w", err)
	}
	out := append([]byte("window.MD2HTML_SEARCH_INDEX = "), data...)
	out = append(out, ';', '\n')
	return out, nil
}

func buildSearchIndexJS(sourceDir string, cfg Config) ([]byte, int, error) {
	documents, err := buildSearchIndex(sourceDir, cfg)
	if err != nil {
		return nil, 0, err
	}
	body, err := renderSearchIndexJS(documents)
	if err != nil {
		return nil, 0, err
	}
	return body, len(documents), nil
}

// extractPlainText extracts plain text from markdown content
func extractPlainText(markdown string) string {
	lines := strings.Split(markdown, "\n")
	var textParts []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and frontmatter
		if line == "" || line == "---" {
			continue
		}

		line = strings.TrimPrefix(line, "######")
		line = strings.TrimPrefix(line, "#####")
		line = strings.TrimPrefix(line, "####")
		line = strings.TrimPrefix(line, "###")
		line = strings.TrimPrefix(line, "##")
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)

		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "+ ")

		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "*", "")
		line = strings.ReplaceAll(line, "`", "")

		// Simple link removal: [text](url) -> text
		for strings.Contains(line, "[") && strings.Contains(line, "]") {
			start := strings.Index(line, "[")
			end := strings.Index(line, "]")
			if start < end {
				linkText := line[start+1 : end]
				urlStart := strings.Index(line[end:], "(")
				if urlStart != -1 {
					urlEnd := strings.Index(line[end+urlStart:], ")")
					if urlEnd != -1 {
						line = line[:start] + linkText + line[end+urlStart+urlEnd+1:]
					} else {
						line = line[:start] + linkText + line[end+1:]
					}
				} else {
					line = line[:start] + linkText + line[end+1:]
				}
			} else {
				break
			}
		}

		if line != "" && !strings.HasPrefix(line, "title:") && !strings.HasPrefix(line, "description:") {
			textParts = append(textParts, line)
		}
	}

	return strings.Join(textParts, " ")
}

// extractBlurb extracts the first paragraph or sentence as a blurb
func extractBlurb(content string) string {
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			content = parts[2]
		}
	}

	lines := strings.Split(content, "\n")
	var textLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "*", "")
		line = strings.ReplaceAll(line, "`", "")

		for strings.Contains(line, "[") && strings.Contains(line, "]") {
			start := strings.Index(line, "[")
			end := strings.Index(line, "]")
			if start < end {
				linkText := line[start+1 : end]
				if urlStart := strings.Index(line[end:], "("); urlStart != -1 {
					urlEnd := strings.Index(line[end+urlStart:], ")")
					if urlEnd != -1 {
						line = line[:start] + linkText + line[end+urlStart+urlEnd+1:]
					} else {
						line = line[:start] + linkText + line[end+1:]
					}
				} else {
					line = line[:start] + linkText + line[end+1:]
				}
			} else {
				break
			}
		}

		if line != "" {
			textLines = append(textLines, line)
		}
		if len(textLines) >= 2 {
			break
		}
	}

	if len(textLines) == 0 {
		return ""
	}

	return strings.Join(textLines, " ")
}

// convertPathToURL converts a file path to a URL
func convertPathToURL(relPath string, htmlExt string) string {
	url := strings.TrimSuffix(relPath, ".md")

	if htmlExt != "" {
		url += "." + htmlExt
	}

	if !strings.HasPrefix(url, "/") {
		url = "/" + url
	}

	return url
}

// truncate truncates text to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	truncated := s[:maxLen]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxLen/2 {
		truncated = s[:lastSpace]
	}

	return truncated + "..."
}
