package md2html

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tmc/md2html/internal/ignorefile"
	"github.com/tmc/md2html/internal/markdownfile"
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
		url := cfg.layout().url(f.RelPath)
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

// defaultDepth is the traversal depth findMarkdownFiles uses when given
// none, so that a zero Config.Depth still finds the pages at the root.
const defaultDepth = 2

// findMarkdownFiles returns the Markdown files under rootDir, descending
// maxDepth levels: 1 is the files in rootDir itself, 2 adds those in its
// subdirectories. A maxDepth below 1 means defaultDepth.
func findMarkdownFiles(rootDir string, maxDepth int) ([]markdownFile, error) {
	if maxDepth < 1 {
		maxDepth = defaultDepth
	}
	var files []markdownFile
	seen := make(map[string]bool) // track real paths to avoid symlink cycles

	// A repository that publishes part of itself says so in an ignore
	// file. Reading it here covers both the server and static output,
	// since everything that enumerates the tree comes through this
	// function.
	ignore, err := ignorefile.Load(rootDir)
	if err != nil {
		slog.Default().Warn("skipping the ignore file", "error", err)
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
				if depth >= maxDepth || isDependencyDir(entry.Name()) {
					continue
				}
				if ignore.Excludes(apparentPath, true) {
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
					slog.Default().Warn("skipping path", "path", apparentPath, "error", err)
				}
				continue
			}

			if depth >= maxDepth {
				continue
			}

			if markdownfile.Is(info.Name()) {
				if ignore.Excludes(apparentPath, false) {
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

	logger.Info("generating static HTML", "source", sourceDir, "output", outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Copy assets before rendering, so anything generated under the same
	// name (llms.txt, the search assets) is what survives.
	assetCount, err := copySourceAssets(sourceDir, outputDir, logger)
	if err != nil {
		logger.Error("error copying assets", "error", err)
	}
	if assetCount > 0 {
		logger.Info("copied assets", "count", assetCount)
	}

	var jsonData any
	if cfg.DataJSON != "" {
		var err error
		jsonData, err = loadJSONFile(cfg.DataJSON)
		if err != nil {
			logger.Error("error loading JSON data file", "error", err, "file", cfg.DataJSON)
		} else {
			logger.Debug("loaded JSON data", "file", cfg.DataJSON)
		}
	}

	var cssContent string
	if cfg.CSS != "" {
		css, err := os.ReadFile(cfg.CSS)
		if err != nil {
			logger.Error("error reading CSS file", "error", err, "file", cfg.CSS)
		} else {
			cssContent = string(css)
			logger.Debug("loaded CSS content", "file", cfg.CSS)
		}
	}

	files, err := findMarkdownFiles(sourceDir, 100) // Use high depth for static generation
	if err != nil {
		return fmt.Errorf("failed to find markdown files: %w", err)
	}

	logger.Info("found markdown files to process", "count", len(files))
	site.links = newSiteLinks(sourceDir, files)

	// With no -index, the root page is chosen as the server chooses it.
	// A README.md or SKILL.md root is then treated as if named by -index,
	// so links and canonical URLs for it name the site root.
	if cfg.Index == "" {
		if name := rootIndexName(sourceDir, files, cfg.Drafts); name != "" && !markdownfile.IsIndex(name) {
			cfg.Index = name
			site.config.Index = name
		}
	}

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
			logger.Error("error loading navigation", "error", err)
		} else if nav != nil && len(nav.Items) > 0 {
			cfg.Title = siteTitle(cfg.Title, sInfo.Name)
			site.config.Title = cfg.Title
			logger.Info("loaded navigation", "pages", len(nav.Flat))
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
		body, n, err := buildSearchIndexJS(sourceDir, cfg, site.includes)
		if err != nil {
			logger.Error("error generating search index", "error", err)
		} else {
			var writeErr error
			assets, writeErr = writeFingerprintedSearchAssets(outputDir, body)
			if writeErr != nil {
				logger.Error("error writing search assets", "error", writeErr)
				assets = map[string]string{}
			} else {
				logger.Info("generated search index", "documents", n)
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
			if cfg.layout().moved(relPath) {
				opts.RawMDURL = "../" + path.Base(normalizeSourcePath(relPath))
			}
		}
		if nav != nil {
			opts.Nav = nav.ForPage(relPath)
		}
		return opts
	}

	var renderErrors []error
	rendered := make(map[string]bool)
	layout := cfg.layout()
	outputs := make(map[string]string)
	for _, file := range files {
		if !cfg.Drafts && isDraft(filepath.Join(sourceDir, file.RelPath)) {
			logger.Debug("skipping draft", "file", file.RelPath)
			continue
		}
		if layout.dirs {
			// The site index is written once, as index.html. A copy
			// under its own name would sit a directory lower, where
			// its links would no longer resolve.
			rel := normalizeSourcePath(file.RelPath)
			if cfg.Index != "" && rel == normalizeSourcePath(cfg.Index) {
				rendered[rel] = true
				continue
			}
			out := layout.file(rel)
			if other, ok := outputs[out]; ok {
				renderErrors = append(renderErrors, fmt.Errorf("process %s: %s is already written for %s", rel, out, other))
				continue
			}
			outputs[out] = rel
		}
		if err := processMarkdownFile(file, sourceDir, outputDir, cssContent, site, pageOptions(file.RelPath)); err != nil {
			logger.Error("error processing file", "error", err, "file", file.RelPath)
			renderErrors = append(renderErrors, fmt.Errorf("process %s: %w", file.RelPath, err))
			continue
		}
		logger.Debug("generated file", "file", file.RelPath)
		rendered[filepath.ToSlash(file.RelPath)] = true
	}

	if cfg.Index != "" {
		indexFile := filepath.Join(sourceDir, cfg.Index)
		if _, err := os.Stat(indexFile); err == nil {
			if err := processIndexFile(indexFile, outputDir, cssContent, site, pageOptions(cfg.Index)); err != nil {
				logger.Error("error processing index file", "error", err, "file", indexFile)
			} else {
				logger.Debug("processed index file", "file", indexFile)
			}
		}
	} else if !slices.ContainsFunc(markdownfile.Exts(), func(ext string) bool { return rendered["index"+ext] }) {
		if err := generateTOCIndex(outputDir, files, cssContent, site, assets); err != nil {
			logger.Error("error generating TOC index", "error", err)
		} else {
			logger.Debug("generated TOC index")
		}
	}

	if cfg.LLMS {
		n, err := generateLLMSFiles(sourceDir, outputDir, files, nav, cfg, site.includes)
		if err != nil {
			logger.Error("error generating llms files", "error", err)
		} else {
			logger.Info("generated llms files", "documents", n)
		}
	}
	// A page that could not include a file was still written, with the
	// reason where the code should be, but the site is not fit to publish.
	for _, e := range site.includes.takeErrors() {
		renderErrors = append(renderErrors, fmt.Errorf("include %s", e))
	}
	if len(renderErrors) > 0 {
		return errors.Join(renderErrors...)
	}

	logger.Info("static HTML generation completed")

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
		slog.Default().Error("error parsing frontmatter", "file", file.RelPath, "error", err)
		doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
	}
	doc = site.includes.expandPage(string(content), doc, file.RelPath)

	htmlContent, err := site.markdownToHTML(promoteTitleHeading(doc), file.RelPath)
	if err != nil {
		return err
	}
	opts.Description = llmsSummary(doc)

	// In the file layout the page chosen as the site index is also
	// written under its own name, so the layout must not send it to
	// index.html here. The dir layout skips that page before this.
	l := cfg.layout()
	l.index = ""
	outputPath := filepath.Join(outputDir, filepath.FromSlash(l.file(file.RelPath)))

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	if cfg.LLMS {
		if err := copyRawMarkdown(sourcePath, outputDir, file.RelPath, site.includes); err != nil {
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
		slog.Default().Error("error parsing frontmatter", "file", indexPath, "error", err)
		doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
	}

	htmlPath := opts.FilePath
	if htmlPath == "" {
		htmlPath = filepath.Base(indexPath)
	}
	doc = site.includes.expandPage(string(content), doc, htmlPath)
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
		if err := copyRawMarkdown(indexPath, outputDir, opts.FilePath, site.includes); err != nil {
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
			url := cfg.layout().url(f.RelPath)
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
var indexNames = []string{"index.md", "index.markdown", "index.mdx", "README.md", "readme.md", "SKILL.md"}

// rootIndexName returns the file among files that is the site's root
// page, as the server would choose it, or "" if there is none.
func rootIndexName(sourceDir string, files []markdownFile, drafts bool) string {
	have := make(map[string]bool)
	for _, f := range files {
		have[filepath.ToSlash(f.RelPath)] = true
	}
	for _, name := range indexNames {
		if have[name] && (drafts || !isDraft(filepath.Join(sourceDir, name))) {
			return name
		}
	}
	return ""
}
