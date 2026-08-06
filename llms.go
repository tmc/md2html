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
	Group   string // navigation group, "" when the page is in none
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

	groups := navGroups(nav)

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
		rel := normalizeSourcePath(f.RelPath)
		pages = append(pages, llmsPage{
			Path:    rel,
			URL:     renderedPathForSource(f.RelPath, cfg.HTMLExt, cfg.Index),
			Title:   title,
			Summary: llmsSummary(doc),
			Content: string(content),
			Group:   groups[rel],
		})
	}
	return pages, nil
}

// navGroups maps each page path to the title of the navigation group
// holding it. Nested groups report the outermost one, which is the level
// llms.txt sections are meant to describe.
func navGroups(nav *Navigation) map[string]string {
	groups := make(map[string]string)
	if nav == nil {
		return groups
	}
	// The two navigation sources shape groups differently. docs.json
	// nests pages inside their group; SUMMARY.md writes "## Name" as a
	// sibling that opens a section running until the next one. Handle
	// both: recurse into children, and carry an open heading sideways.
	var walk func(items []*NavItem, group string)
	walk = func(items []*NavItem, group string) {
		open := group
		for _, item := range items {
			switch {
			case item.IsSep:
				open = group
			case item.IsGroup:
				name := group
				if name == "" {
					name = item.Title
				}
				open = name
				walk(item.Children, name)
			default:
				if item.Path != "" && open != "" {
					groups[normalizeSourcePath(item.Path)] = open
				}
				walk(item.Children, open)
			}
		}
	}
	walk(nav.Items, "")
	return groups
}

// buildLLMSSummary renders the llms.txt index described at llmstxt.org:
// an H1 naming the site, an optional blockquote summary, then H2
// sections of Markdown links with a one-line description each. The
// format is Markdown on purpose — a model reading it should not have to
// guess where a URL ends.
func buildLLMSSummary(cfg Config, pages []llmsPage) string {
	var b strings.Builder
	title := strings.TrimSpace(cfg.Title)
	if title == "" || title == defaultTitle {
		if len(pages) > 0 {
			if h := firstHeading(pages[0].Content); h != "" {
				title = h
			} else {
				title = pages[0].Title
			}
		}
	}
	if title == "" {
		title = "Documentation"
	}
	fmt.Fprintf(&b, "# %s\n", title)

	// The first page's own summary describes the site better than
	// anything md2html could synthesise.
	if len(pages) > 0 && pages[0].Summary != "" {
		fmt.Fprintf(&b, "\n> %s\n", pages[0].Summary)
	}

	// Sections follow navigation order. A tree with no groups still needs
	// one heading for the links to sit under, and calling that "Other"
	// would imply a main section that does not exist.
	ungrouped := "Other"
	grouped := false
	for _, p := range pages {
		if p.Group != "" {
			grouped = true
			break
		}
	}
	if !grouped {
		ungrouped = "Pages"
	}

	var sections []string
	bySection := make(map[string][]llmsPage)
	for _, p := range pages {
		group := p.Group
		if group == "" {
			group = ungrouped
		}
		if _, seen := bySection[group]; !seen {
			sections = append(sections, group)
		}
		bySection[group] = append(bySection[group], p)
	}

	for _, name := range sections {
		fmt.Fprintf(&b, "\n## %s\n\n", name)
		for _, p := range bySection[name] {
			fmt.Fprintf(&b, "- [%s](%s)", p.Title, p.URL)
			if p.Summary != "" {
				fmt.Fprintf(&b, ": %s", p.Summary)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func writeLLMSSummary(outputDir string, cfg Config, pages []llmsPage) error {
	return os.WriteFile(filepath.Join(outputDir, "llms.txt"), []byte(buildLLMSSummary(cfg, pages)), 0644)
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
	base := path.Base(normalizeSourcePath(relPath))
	if s := skillName(base, doc); s != "" {
		return s
	}
	if h := firstHeading(doc.Content); h != "" {
		return h
	}
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
