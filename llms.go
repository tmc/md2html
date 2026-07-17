package md2html

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var llmsFullMaxBytes = 1 << 20

type llmsPage struct {
	Path    string
	URL     string
	Title   string
	Summary string
	Content string
}

func generateLLMSFiles(sourceDir, outputDir string, files []markdownFile, nav *Navigation, cfg Config) (int, error) {
	pages, err := collectLLMSPages(sourceDir, files, nav, cfg)
	if err != nil {
		return 0, err
	}
	if err := writeLLMSSummary(outputDir, cfg, pages); err != nil {
		return 0, err
	}
	if err := writeLLMSFull(outputDir, pages, llmsFullMaxBytes); err != nil {
		return 0, err
	}
	return len(pages), nil
}

func collectLLMSPages(sourceDir string, files []markdownFile, nav *Navigation, cfg Config) ([]llmsPage, error) {
	byPath := make(map[string]markdownFile)
	for _, f := range files {
		rel := normalizeSourcePath(f.RelPath)
		if rel == "" || strings.EqualFold(rel, "SUMMARY.md") || isDraft(filepath.Join(sourceDir, f.RelPath)) {
			continue
		}
		byPath[rel] = f
	}

	var ordered []markdownFile
	used := make(map[string]bool)
	if nav != nil {
		for _, item := range nav.Flat {
			rel := normalizeSourcePath(item.Path)
			f, ok := byPath[rel]
			if !ok {
				continue
			}
			ordered = append(ordered, f)
			used[rel] = true
		}
	}

	var rest []string
	for rel := range byPath {
		if !used[rel] {
			rest = append(rest, rel)
		}
	}
	sort.Strings(rest)
	for _, rel := range rest {
		ordered = append(ordered, byPath[rel])
	}

	pages := make([]llmsPage, 0, len(ordered))
	for _, f := range ordered {
		content, err := os.ReadFile(filepath.Join(sourceDir, f.RelPath))
		if err != nil {
			return nil, fmt.Errorf("read markdown: %w", err)
		}
		doc, err := parseFrontmatter(string(content))
		if err != nil {
			doc = DocumentData{Content: string(content), Frontmatter: map[string]any{}}
		}
		title := llmsTitle(f.RelPath, doc)
		pages = append(pages, llmsPage{
			Path:    normalizeSourcePath(f.RelPath),
			URL:     renderedPathForSource(f.RelPath, cfg.HTMLExt, cfg.Index),
			Title:   title,
			Summary: llmsSummary(doc),
			Content: string(content),
		})
	}
	return pages, nil
}

func writeLLMSSummary(outputDir string, cfg Config, pages []llmsPage) error {
	var b strings.Builder
	title := strings.TrimSpace(cfg.Title)
	if title == "" && len(pages) > 0 {
		title = firstHeading(pages[0].Content)
		if title == "" {
			title = pages[0].Title
		}
	}
	if title == "" {
		title = "Documentation"
	}
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, p := range pages {
		b.WriteString(p.URL)
		b.WriteString(" — ")
		b.WriteString(p.Title)
		if p.Summary != "" {
			b.WriteString(" — ")
			b.WriteString(p.Summary)
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return os.WriteFile(filepath.Join(outputDir, "llms.txt"), []byte(b.String()), 0644)
}

func writeLLMSFull(outputDir string, pages []llmsPage, maxBytes int) error {
	if maxBytes <= 0 {
		maxBytes = llmsFullMaxBytes
	}
	var parts [][]byte
	var cur bytes.Buffer
	for _, p := range pages {
		section := []byte("--- " + p.URL + "\n\n" + p.Content)
		extra := 0
		if cur.Len() > 0 {
			extra = 2
		}
		if cur.Len() > 0 && cur.Len()+extra+len(section) > maxBytes {
			parts = append(parts, append([]byte(nil), cur.Bytes()...))
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.Write(section)
	}
	if cur.Len() > 0 || len(parts) == 0 {
		parts = append(parts, append([]byte(nil), cur.Bytes()...))
	}
	if len(parts) == 1 {
		return os.WriteFile(filepath.Join(outputDir, "llms-full.txt"), parts[0], 0644)
	}
	for i, part := range parts {
		name := fmt.Sprintf("llms-full-%03d.txt", i+1)
		header := fmt.Sprintf("llms-full part %d of %d\n\n", i+1, len(parts))
		if err := os.WriteFile(filepath.Join(outputDir, name), append([]byte(header), part...), 0644); err != nil {
			return err
		}
	}
	return nil
}

func llmsTitle(relPath string, doc DocumentData) string {
	if s := firstFrontmatterString(doc.Frontmatter, "title"); s != "" {
		return s
	}
	if h := firstHeading(doc.Content); h != "" {
		return h
	}
	base := path.Base(normalizeSourcePath(relPath))
	return strings.TrimSuffix(base, path.Ext(base))
}

func llmsSummary(doc DocumentData) string {
	if s := firstFrontmatterString(doc.Frontmatter, "description"); s != "" {
		return truncateRunes(singleLine(s), 160)
	}
	return truncateRunes(singleLine(firstParagraph(doc.Content)), 160)
}

var headingPattern = regexp.MustCompile(`^#\s+(.+?)\s*$`)

func firstHeading(markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		m := headingPattern.FindStringSubmatch(strings.TrimSpace(line))
		if m != nil {
			return plainMarkdown(m[1])
		}
	}
	return ""
}

func firstParagraph(markdown string) string {
	var lines []string
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return plainMarkdown(strings.Join(lines, " "))
}

func plainMarkdown(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "`", "")
	return linkPattern.ReplaceAllString(s, "$1")
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	r = r[:max]
	cut := max
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == ' ' {
			cut = i
			break
		}
	}
	if cut < max/2 {
		cut = max
	}
	return strings.TrimSpace(string(r[:cut])) + "..."
}

func rawMarkdownURL(relPath string) string {
	return "./" + path.Base(normalizeSourcePath(relPath))
}

func copyRawMarkdown(sourcePath, outputDir, relPath string) error {
	dest := filepath.Join(outputDir, filepath.FromSlash(normalizeSourcePath(relPath)))
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}
