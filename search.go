package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SearchDocument represents a document in the search index
type SearchDocument struct {
	Title    string `json:"title"`
	Text     string `json:"text"`
	Category string `json:"category"`
	URL      string `json:"url"`
	Blurb    string `json:"blurb"`
	Type     string `json:"type"`
}

// generateSearchIndex creates a JSON search index for all markdown files
func generateSearchIndex(sourceDir, outputDir string, cfg Config) error {
	var documents []SearchDocument

	// Walk through all markdown files
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-markdown files
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Read the file
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", path, err)
		}

		// Parse frontmatter
		doc, err := parseFrontmatter(string(content))
		if err != nil {
			// If no frontmatter, use raw content
			doc = DocumentData{
				Content:     string(content),
				Frontmatter: make(map[string]interface{}),
			}
		}

		// Extract title from frontmatter or filename
		title := ""
		if t, ok := doc.Frontmatter["title"].(string); ok {
			title = t
		} else {
			title = strings.TrimSuffix(filepath.Base(path), ".md")
		}

		// Extract blurb/description
		blurb := ""
		if b, ok := doc.Frontmatter["description"].(string); ok {
			blurb = b
		} else if b, ok := doc.Frontmatter["blurb"].(string); ok {
			blurb = b
		} else {
			// Extract first paragraph as blurb
			blurb = extractBlurb(doc.Content)
		}

		// Extract category from path
		relPath, _ := filepath.Rel(sourceDir, path)
		category := filepath.Dir(relPath)
		if category == "." {
			category = ""
		}

		// Convert path to URL
		url := convertPathToURL(relPath, cfg.HTMLExt)

		// Extract plain text from markdown
		plainText := extractPlainText(doc.Content)

		// Determine document type
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
		return fmt.Errorf("error walking directory: %w", err)
	}

	// Write search index
	indexPath := filepath.Join(outputDir, "search-index.json")
	indexData, err := json.MarshalIndent(documents, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling search index: %w", err)
	}

	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		return fmt.Errorf("error writing search index: %w", err)
	}

	fmt.Printf("Generated search index: %d documents\n", len(documents))
	return nil
}

// extractPlainText extracts plain text from markdown content
func extractPlainText(markdown string) string {
	// Simple approach: remove markdown syntax and extract text
	lines := strings.Split(markdown, "\n")
	var textParts []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and frontmatter
		if line == "" || line == "---" {
			continue
		}

		// Remove heading markers
		line = strings.TrimPrefix(line, "######")
		line = strings.TrimPrefix(line, "#####")
		line = strings.TrimPrefix(line, "####")
		line = strings.TrimPrefix(line, "###")
		line = strings.TrimPrefix(line, "##")
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)

		// Remove list markers
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "+ ")

		// Remove markdown formatting
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
	// Remove frontmatter if present
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			content = parts[2]
		}
	}

	// Remove markdown headings
	lines := strings.Split(content, "\n")
	var textLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Remove markdown links, bold, italic
		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "*", "")
		line = strings.ReplaceAll(line, "`", "")

		// Simple link removal
		for strings.Contains(line, "[") && strings.Contains(line, "]") {
			start := strings.Index(line, "[")
			end := strings.Index(line, "]")
			if start < end {
				linkText := line[start+1 : end]
				// Remove URL part if present
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
	// Remove .md extension
	url := strings.TrimSuffix(relPath, ".md")

	// Add HTML extension if configured
	if htmlExt != "" {
		url += "." + htmlExt
	}

	// Ensure it starts with /
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

	// Try to break at word boundary
	truncated := s[:maxLen]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxLen/2 {
		truncated = s[:lastSpace]
	}

	return truncated + "..."
}
