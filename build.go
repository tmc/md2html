package md2html

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// listingTitle names a generated directory listing. The label is the
// directory's path relative to the served root; the filesystem location of
// the root is not part of the served page.
func listingTitle(relDir string) string {
	relDir = filepath.ToSlash(relDir)
	if relDir == "" || relDir == "." {
		return "Index of /"
	}
	return "Index of /" + strings.TrimSuffix(relDir, "/")
}

// generateDirectoryListing renders a Markdown listing of the Markdown files
// under dir. relDir is dir's path relative to the served root and is used for
// the heading. A directory's index file, if any, is served in place of the
// listing rather than being concatenated with it.
func generateDirectoryListing(cfg Config, dir, relDir string) (string, error) {
	files, err := findMarkdownFiles(dir, cfg.Depth)
	if err != nil {
		return "", err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})

	var buf strings.Builder

	buf.WriteString(fmt.Sprintf("# %s\n\n", listingTitle(relDir)))

	if len(files) == 0 {
		buf.WriteString("*No markdown files found.*\n")
		return buf.String(), nil
	}

	for _, f := range files {
		url := renderedPathForSource(f.RelPath, cfg.HTMLExt, cfg.Index)
		buf.WriteString(fmt.Sprintf("- [%s](%s) (%d bytes, %s)\n",
			f.RelPath, url, f.Size, f.ModTime.Format("2006-01-02 15:04")))
	}

	return buf.String(), nil
}

type markdownFile struct {
	RelPath string
	Size    int64
	ModTime time.Time
}

// minDepth is the smallest traversal depth findMarkdownFiles honors, so
// that a zero Config.Depth still finds the pages at the root.
const minDepth = 2

func findMarkdownFiles(rootDir string, maxDepth int) ([]markdownFile, error) {
	maxDepth = max(maxDepth, minDepth)
	var files []markdownFile
	seen := make(map[string]bool) // track real paths to avoid symlink cycles

	// A repository that publishes part of itself says so in an ignore
	// file. Reading it here covers both the server and static output,
	// since everything that enumerates the tree comes through this
	// function.
	ignore, err := loadIgnoreSet(rootDir)
	if err != nil {
		slog.Default().Warn("Skipping the ignore file", "error", err)
	}

	// walkDir walks a directory rooted at realDir, mapping discovered paths
	// to appear under apparentDir relative to rootDir.
	var walkDir func(realDir, apparentDir string) error
	walkDir = func(realDir, apparentDir string) error {
		entries, err := os.ReadDir(realDir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			realPath := filepath.Join(realDir, entry.Name())
			apparentPath := filepath.Join(apparentDir, entry.Name())

			info, err := entry.Info()
			if err != nil {
				continue
			}

			if info.Mode()&os.ModeSymlink != 0 {
				resolved, err := filepath.EvalSymlinks(realPath)
				if err != nil {
					continue // skip broken symlinks
				}
				info, err = os.Stat(resolved)
				if err != nil {
					continue
				}
				realPath = resolved
			}

			relPath, err := filepath.Rel(rootDir, apparentPath)
			if err != nil {
				continue
			}
			depth := strings.Count(relPath, string(filepath.Separator))

			if info.IsDir() {
				if depth >= maxDepth {
					continue
				}
				if ignore.excludes(apparentPath, true) {
					continue
				}
				real, err := filepath.EvalSymlinks(apparentPath)
				if err != nil {
					real = realPath
				}
				if seen[real] {
					continue // avoid cycles
				}
				seen[real] = true
				if err := walkDir(realPath, apparentPath); err != nil {
					// Skip subdirectories we cannot read (e.g. permission
					// denied on system directories like .Trashes) rather than
					// aborting the entire listing.
					slog.Default().Warn("Skipping path", "path", apparentPath, "error", err)
				}
				continue
			}

			if depth >= maxDepth {
				continue
			}

			name := strings.ToLower(info.Name())
			if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown") {
				if ignore.excludes(apparentPath, false) {
					continue
				}
				files = append(files, markdownFile{
					RelPath: relPath,
					Size:    info.Size(),
					ModTime: info.ModTime(),
				})
			}
		}
		return nil
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	rootDir = absRoot
	seen[absRoot] = true
	err = walkDir(rootDir, rootDir)

	return files, err
}

// sourceDir returns the directory a static build reads from. A source
// that names nothing, or stdin, has no directory of its own and is
// rooted at the base its configuration was resolved against.
func (s *preparedSite) sourceDir() string {
	if s.config.Source == "" || s.config.Source == "-" {
		if s.base != "" {
			return s.base
		}
		return "."
	}
	return s.config.Source
}

func (site *preparedSite) generateStaticHTML(ctx context.Context, logger *slog.Logger) error {
	cfg := site.config
	sourceDir := site.sourceDir()

	outputDir := cfg.HTML

	logger.Info("Generating static HTML", "source", sourceDir, "output", outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Copy assets before rendering, so anything generated under the same
	// name (llms.txt, the search assets) is what survives.
	assetCount, err := copySourceAssets(sourceDir, outputDir, logger)
	if err != nil {
		logger.Error("Error copying assets", "error", err)
	}
	if assetCount > 0 {
		logger.Info("Copied assets", "count", assetCount)
	}

	var jsonData any
	if cfg.DataJSON != "" {
		var err error
		jsonData, err = loadJSONFile(cfg.DataJSON)
		if err != nil {
			logger.Error("Error loading JSON data file", "error", err, "file", cfg.DataJSON)
		} else {
			logger.Debug("Loaded JSON data", "file", cfg.DataJSON)
		}
	}

	var cssContent string
	if cfg.CSS != "" {
		css, err := os.ReadFile(cfg.CSS)
		if err != nil {
			logger.Error("Error reading CSS file", "error", err, "file", cfg.CSS)
		} else {
			cssContent = string(css)
			logger.Debug("Loaded CSS content", "file", cfg.CSS)
		}
	}

	files, err := findMarkdownFiles(sourceDir, 100) // Use high depth for static generation
	if err != nil {
		return fmt.Errorf("failed to find markdown files: %w", err)
	}

	logger.Info("Found markdown files to process", "count", len(files))
	site.links = newSiteLinks(sourceDir, files)

	var nav *Navigation
	var sInfo siteInfo
	if cfg.Nav {
		htmlExt := ""
		if cfg.HTMLExt != "" {
			htmlExt = "." + cfg.HTMLExt
		}
		var err error
		nav, sInfo, err = navigationForDir(sourceDir, htmlExt)
		if err != nil {
			logger.Error("Error loading navigation", "error", err)
		} else if nav != nil && len(nav.Items) > 0 {
			cfg.Title = siteTitle(cfg.Title, sInfo.Name)
			site.config.Title = cfg.Title
			logger.Info("Loaded navigation", "pages", len(nav.Flat))
			sInfo.Stars = site.repoStars(ctx, sInfo.Repo, logger)
		}
	}
	lastUpdated := map[string]string{}
	if times, err := gitLastUpdated(ctx, sourceDir, files); err == nil {
		lastUpdated = times
	} else if cfg.Verbose {
		logger.Debug("git metadata unavailable", "error", err)
	}
	assets := map[string]string{}
	if site.jsonSpecBundle != "" {
		if err := writeJSONSpecAsset(outputDir); err != nil {
			return err
		}
	}
	if cfg.Search {
		body, n, err := buildSearchIndexJS(sourceDir, cfg)
		if err != nil {
			logger.Error("Error generating search index", "error", err)
		} else {
			var writeErr error
			assets, writeErr = writeFingerprintedSearchAssets(outputDir, body)
			if writeErr != nil {
				logger.Error("Error writing search assets", "error", writeErr)
				assets = map[string]string{}
			} else {
				logger.Info("Generated search index", "documents", n)
			}
		}
	}

	// pageOptions returns the options for rendering the page whose source
	// is relPath under sourceDir.
	pageOptions := func(relPath string) RenderOptions {
		opts := RenderOptions{
			SiteTitle:   cfg.Title,
			Data:        jsonData,
			FilePath:    relPath,
			EditURL:     editURL(cfg.EditURL, relPath),
			LastUpdated: lastUpdated[filepath.ToSlash(relPath)],
			Assets:      assets,
			Accent:      sInfo.Accent,
			AccentDark:  sInfo.AccentDark,
			Repo:        sInfo.Repo,
			RepoURL:     sInfo.RepoURL,
			NavLinks:    sInfo.Links,
			Stars:       sInfo.Stars,
			ShowStars:   cfg.Stars,
		}
		if cfg.LLMS {
			opts.RawMDURL = rawMarkdownURL(relPath)
		}
		if nav != nil {
			opts.Nav = nav.ForPage(relPath)
		}
		return opts
	}

	var renderErrors []error
	rendered := make(map[string]bool)
	for _, file := range files {
		if !cfg.Drafts && isDraft(filepath.Join(sourceDir, file.RelPath)) {
			logger.Debug("Skipping draft", "file", file.RelPath)
			continue
		}
		if err := processMarkdownFile(file, sourceDir, outputDir, cssContent, site, pageOptions(file.RelPath)); err != nil {
			logger.Error("Error processing file", "error", err, "file", file.RelPath)
			renderErrors = append(renderErrors, fmt.Errorf("process %s: %w", file.RelPath, err))
			continue
		}
		logger.Debug("Generated file", "file", file.RelPath)
		rendered[filepath.ToSlash(file.RelPath)] = true
	}

	if cfg.Index != "" {
		indexFile := filepath.Join(sourceDir, cfg.Index)
		if _, err := os.Stat(indexFile); err == nil {
			if err := processIndexFile(indexFile, outputDir, cssContent, site, pageOptions(cfg.Index)); err != nil {
				logger.Error("Error processing index file", "error", err, "file", indexFile)
			} else {
				logger.Debug("Processed index file", "file", indexFile)
			}
		}
	} else {
		// With no -index, the root page is chosen as the server chooses
		// it. An index.md or index.markdown is already index.html.
		switch name := rootIndexName(rendered); name {
		case "index.md", "index.markdown":
		case "":
			if err := generateTOCIndex(outputDir, files, cssContent, site, assets); err != nil {
				logger.Error("Error generating TOC index", "error", err)
			} else {
				logger.Debug("Generated TOC index")
			}
		default:
			indexFile := filepath.Join(sourceDir, name)
			if err := processIndexFile(indexFile, outputDir, cssContent, site, pageOptions(name)); err != nil {
				logger.Error("Error processing index file", "error", err, "file", indexFile)
			} else {
				logger.Debug("Processed index file", "file", indexFile)
			}
		}
	}

	if cfg.LLMS {
		n, err := generateLLMSFiles(sourceDir, outputDir, files, nav, cfg)
		if err != nil {
			logger.Error("Error generating llms files", "error", err)
		} else {
			logger.Info("Generated llms files", "documents", n)
		}
	}
	if len(renderErrors) > 0 {
		return errors.Join(renderErrors...)
	}

	logger.Info("Static HTML generation completed")

	return nil
}

// isDraft returns true if the file's frontmatter has draft: true.
func isDraft(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	doc, err := parseFrontmatter(string(content))
	if err != nil {
		return false
	}
	if draft, ok := doc.Frontmatter["draft"].(bool); ok {
		return draft
	}
	return false
}

func processMarkdownFile(file markdownFile, sourceDir, outputDir, cssContent string, site *preparedSite, opts RenderOptions) error {
	cfg := site.config
	sourcePath := filepath.Join(sourceDir, file.RelPath)

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	doc, err := parseFrontmatter(string(content))
	if err != nil {
		slog.Default().Error("Error parsing frontmatter", "file", file.RelPath, "error", err)
		doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
	}

	htmlContent, err := site.markdownToHTML(promoteTitleHeading(doc), file.RelPath)
	if err != nil {
		return err
	}
	opts.Description = llmsSummary(doc)

	baseName := strings.TrimSuffix(file.RelPath, filepath.Ext(file.RelPath))
	outputPath := baseName
	if cfg.HTMLExt != "" {
		outputPath = baseName + "." + cfg.HTMLExt
	}
	outputPath = filepath.Join(outputDir, outputPath)

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	if cfg.LLMS {
		if err := copyRawMarkdown(sourcePath, outputDir, file.RelPath); err != nil {
			return err
		}
	}

	title := pageTitle(doc.Frontmatter, file.RelPath, cfg.Title)

	finalHTML, err := site.renderTemplate(htmlContent, title, cssContent, false, doc.Frontmatter, opts)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(finalHTML), 0644)
}

func pageTitle(frontmatter map[string]any, filePath, fallback string) string {
	if title, ok := frontmatter["title"].(string); ok && title != "" {
		return title
	}
	name := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if name != "" {
		return name
	}
	return fallback
}

// promoteTitleHeading returns the document body with its frontmatter
// presentation applied: the title prepended as an H1 when the body does
// not already open with one, and the description inserted as the lede
// paragraph under that opening H1. Mintlify pages carry both in
// frontmatter alone, so without this they would open with body text.
func promoteTitleHeading(doc DocumentData) string {
	title := strings.TrimSpace(firstFrontmatterString(doc.Frontmatter, "title"))
	desc := strings.TrimSpace(firstFrontmatterString(doc.Frontmatter, "description"))
	// A body that already states the description keeps its own copy.
	if desc != "" && strings.Contains(doc.Content, desc) {
		desc = ""
	}

	lines := strings.Split(doc.Content, "\n")
	first := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			first = i
			break
		}
	}

	hasH1 := first >= 0 && (strings.HasPrefix(strings.TrimSpace(lines[first]), "# ") || strings.TrimSpace(lines[first]) == "#")
	switch {
	case hasH1:
		if desc == "" {
			return doc.Content
		}
		return strings.Join(lines[:first+1], "\n") + "\n\n" + desc + "\n" + strings.Join(lines[first+1:], "\n")
	case title != "":
		head := "# " + title + "\n\n"
		if desc != "" {
			head += desc + "\n\n"
		}
		return head + doc.Content
	}
	return doc.Content
}

// documentTitle names a rendered page. Frontmatter wins, then the document's
// first heading, then the file name, then fallback. The heading is preferred
// over the file name because it is what the reader sees at the top of the
// page.
func documentTitle(doc DocumentData, filePath, fallback string) string {
	if title, ok := doc.Frontmatter["title"].(string); ok && title != "" {
		return title
	}
	if heading := firstHeading(doc.Content); heading != "" {
		return heading
	}
	if name := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath)); name != "" && name != "." {
		return name
	}
	return fallback
}

func processIndexFile(indexPath, outputDir, cssContent string, site *preparedSite, opts RenderOptions) error {
	cfg := site.config
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	doc, err := parseFrontmatter(string(content))
	if err != nil {
		slog.Default().Error("Error parsing frontmatter", "file", indexPath, "error", err)
		doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
	}

	htmlPath := opts.FilePath
	if htmlPath == "" {
		htmlPath = filepath.Base(indexPath)
	}
	htmlContent, err := site.markdownToHTML(promoteTitleHeading(doc), htmlPath)
	if err != nil {
		return err
	}
	opts.Description = llmsSummary(doc)

	title := cfg.Title
	if docTitle, ok := doc.Frontmatter["title"].(string); ok && docTitle != "" {
		title = docTitle
	}

	finalHTML, err := site.renderTemplate(htmlContent, title, cssContent, false, doc.Frontmatter, opts)
	if err != nil {
		return err
	}

	indexOutputPath := filepath.Join(outputDir, "index.html")
	if cfg.LLMS && opts.FilePath != "" {
		if err := copyRawMarkdown(indexPath, outputDir, opts.FilePath); err != nil {
			return err
		}
	}
	return os.WriteFile(indexOutputPath, []byte(finalHTML), 0644)
}

func generateTOCIndex(outputDir string, files []markdownFile, cssContent string, site *preparedSite, assets map[string]string) error {
	cfg := site.config
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("# %s\n\n", listingTitle("")))

	if len(files) == 0 {
		buf.WriteString("*No markdown files found.*\n")
	} else {
		for _, f := range files {
			url := renderedPathForSource(f.RelPath, cfg.HTMLExt, cfg.Index)
			buf.WriteString(fmt.Sprintf("- [%s](%s) (%d bytes, %s)\n",
				f.RelPath, url, f.Size, f.ModTime.Format("2006-01-02 15:04")))
		}
	}

	htmlContent, err := site.markdownToHTML(buf.String(), "")
	if err != nil {
		return err
	}

	doc := DocumentData{Content: buf.String(), Frontmatter: make(map[string]any)}
	finalHTML, err := site.renderTemplate(htmlContent, listingTitle(""), cssContent, false, doc.Frontmatter, RenderOptions{Assets: assets, SiteTitle: cfg.Title})
	if err != nil {
		return err
	}

	indexOutputPath := filepath.Join(outputDir, "index.html")
	return os.WriteFile(indexOutputPath, []byte(finalHTML), 0644)
}

// indexNames are the file names that stand for the directory holding
// them, in order of preference.
var indexNames = []string{"index.md", "index.markdown", "README.md", "readme.md", "SKILL.md"}

// rootIndexName returns the first of indexNames at the top of the source
// tree that is among the rendered pages, or "" if there is none.
func rootIndexName(rendered map[string]bool) string {
	for _, name := range indexNames {
		if rendered[name] {
			return name
		}
	}
	return ""
}
