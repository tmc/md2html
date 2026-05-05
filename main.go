package md2html

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Source            string // file, directory, or "-" for stdin
	HTTP              string
	HTML              string
	Open              bool
	Verbose           bool
	Title             string
	CSS               string
	Depth             int
	TOC               bool
	AllowUnsafe       bool
	TemplateDir       string
	DataJSON          string
	RenderFrontmatter bool
	Index             string
	HTMLExt           string
	Versions          bool
	VersionPattern    string
	VersionBranches   bool
	VersionDefault    string
	Search            bool
	LLMS              bool
	SiteURL           string
	EditURL           string

	// Vet enables non-blocking mdvet checks. When true, [Run] reports
	// any diagnostics it finds to the logger (or stderr) at warn level
	// before continuing. Diagnostics never cause Run to fail.
	Vet bool
	// VetChecks is a comma-separated list of mdvet check names to run
	// when Vet is true. Empty means run every check. Unknown names are
	// logged and skipped — vet must never block rendering.
	VetChecks string

	// JSONSpecPrefixes is a comma-separated list of type-discriminator
	// prefixes (for example "ascf/") that mark fenced JSON blocks for
	// schema-badge enrichment. When empty, the jsonspec extension is a
	// no-op.
	JSONSpecPrefixes string
	// JSONSpecBadgeURL is a printf-style URL template used to link each
	// badge to its schema documentation page. %s is replaced with the
	// discriminator suffix (for example "hypothesis" from "ascf/hypothesis").
	JSONSpecBadgeURL string
	// JSONSpecBadgeLabel is a printf-style template for the badge label
	// text. %s is replaced with the discriminator suffix. Defaults to
	// a clipboard icon plus the suffix when empty.
	JSONSpecBadgeLabel string
	// JSONSpecSchemas is a directory of *.schema.json files. When set,
	// the server loads each schema into a bundle (keyed by filename
	// stem) and exposes it to the rendered page so client-side code can
	// attach tooltips to fields in tagged JSON blocks.
	JSONSpecSchemas string
}

// NewFlagSet returns a FlagSet configured for the md2html CLI.
func NewFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.String("http", "", "HTTP server bind address")
	fs.String("html", "", "output directory for static HTML generation (disables server mode)")
	fs.Bool("open", false, "automatically open browser")
	fs.Bool("v", false, "verbose logging")
	fs.String("title", "Markdown Preview", "HTML title")
	fs.String("css", "", "path to custom CSS file")
	fs.Int("depth", 2, "directory traversal depth for listings (minimum 2)")
	fs.Bool("toc", false, "generate table of contents")
	fs.Bool("allow-unsafe", false, "allow unsafe HTML in markdown (use with caution)")
	fs.String("templates", "", "path to custom template directory (overrides embedded templates)")
	fs.String("data-json", "", "path to JSON file to load as template data (available as .Data)")
	fs.Bool("render-frontmatter", false, "render YAML frontmatter as part of the document content")
	fs.String("index", "", "default file to serve for root path (e.g., README.md, index.md)")
	fs.String("html-ext", "", "file extension for generated HTML files (e.g., 'html' for .html, empty for no extension except index.html)")
	fs.Bool("versions", false, "enable versioned documentation using git tags/refs")
	fs.String("version-pattern", "*", "git tag pattern to match for versions (e.g., 'v*', 'release-*')")
	fs.Bool("version-branches", false, "include branches as versions alongside tags")
	fs.String("version-default", "", "default version to show (empty = current/latest)")
	fs.Bool("search", false, "enable client-side search")
	fs.Bool("llms", false, "emit llms.txt, llms-full.txt, and raw markdown links in static output")
	fs.String("site-url", "", "canonical base URL for generated pages")
	fs.String("edit-url", "", "URL template for edit links; {path} is replaced with the source path")
	fs.String("jsonspec-prefixes", "", "comma-separated JSON type-discriminator prefixes to enrich (e.g. 'ascf/')")
	fs.String("jsonspec-badge-url", "", "URL template for schema badges; %s is the discriminator suffix (e.g. 'schemas.html#%s')")
	fs.String("jsonspec-badge-label", "", "label template for schema badges; %s is the discriminator suffix")
	fs.String("jsonspec-schemas", "", "directory of *.schema.json files; loads a bundle used by client-side tooltips")
	fs.Bool("vet", false, "run mdvet checks on source markdown and report diagnostics (does not block rendering)")
	fs.String("vet-checks", "", "comma-separated mdvet check names to run with -vet (default: all)")
	return fs
}

// ConfigFromFlags creates a Config from an initialized FlagSet.
func ConfigFromFlags(fs *flag.FlagSet) Config {
	return Config{
		HTTP:              fs.Lookup("http").Value.String(),
		HTML:              fs.Lookup("html").Value.String(),
		Open:              fs.Lookup("open").Value.String() == "true",
		Verbose:           fs.Lookup("v").Value.String() == "true",
		Title:             fs.Lookup("title").Value.String(),
		CSS:               fs.Lookup("css").Value.String(),
		Depth:             int(fs.Lookup("depth").Value.(flag.Getter).Get().(int)),
		TOC:               fs.Lookup("toc").Value.String() == "true",
		AllowUnsafe:       fs.Lookup("allow-unsafe").Value.String() == "true",
		TemplateDir:       fs.Lookup("templates").Value.String(),
		DataJSON:          fs.Lookup("data-json").Value.String(),
		RenderFrontmatter: fs.Lookup("render-frontmatter").Value.String() == "true",
		Index:             fs.Lookup("index").Value.String(),
		HTMLExt:           fs.Lookup("html-ext").Value.String(),
		Versions:          fs.Lookup("versions").Value.String() == "true",
		VersionPattern:    fs.Lookup("version-pattern").Value.String(),
		VersionBranches:   fs.Lookup("version-branches").Value.String() == "true",
		VersionDefault:    fs.Lookup("version-default").Value.String(),
		Search:            fs.Lookup("search").Value.String() == "true",
		LLMS:              fs.Lookup("llms").Value.String() == "true",
		SiteURL:           fs.Lookup("site-url").Value.String(),
		EditURL:           fs.Lookup("edit-url").Value.String(),

		JSONSpecPrefixes:   fs.Lookup("jsonspec-prefixes").Value.String(),
		JSONSpecBadgeURL:   fs.Lookup("jsonspec-badge-url").Value.String(),
		JSONSpecBadgeLabel: fs.Lookup("jsonspec-badge-label").Value.String(),
		JSONSpecSchemas:    fs.Lookup("jsonspec-schemas").Value.String(),

		Vet:       fs.Lookup("vet").Value.String() == "true",
		VetChecks: fs.Lookup("vet-checks").Value.String(),
	}
}

func Run(ctx context.Context, cfg Config, logger *slog.Logger, out io.Writer, args []string) error {
	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Handle positional arguments
	if len(args) > 0 {
		cfg.Source = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("too many positional arguments")
	}

	// Configure logger level based on verbose flag
	if cfg.Verbose {
		// Create a new logger with debug level when verbose is enabled
		opts := &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
		handler := slog.NewTextHandler(os.Stderr, opts)
		logger = slog.New(handler)
	}

	// Run mdvet checks before any rendering. Diagnostics are reported
	// to the logger but never cause Run to fail.
	if cfg.Vet {
		runVet(cfg, logger)
	}

	// TODO: clean up handling stdin and choosing between modes

	// If -html flag is provided, generate static HTML
	if cfg.HTML != "" {
		// Default to .html extension for static builds so files are
		// served with the correct Content-Type by standard HTTP servers.
		if cfg.HTMLExt == "" {
			cfg.HTMLExt = "html"
		}
		return generateStaticHTML(ctx, cfg, logger)
	}

	// If -http flag is provided, run server
	if cfg.HTTP != "" {
		logger.Info("Starting server", "address", cfg.HTTP)
		err := runServer(ctx, cfg, logger)
		// Don't treat context cancellation as an error (graceful shutdown)
		if err == context.Canceled {
			return nil
		}
		return err
	}

	// If source is provided but no mode specified, convert to HTML and output to stdout
	if cfg.Source != "" && cfg.Source != "." {
		// Read the markdown file
		content, err := os.ReadFile(cfg.Source)
		if err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}

		// Convert to HTML
		doc, err := parseFrontmatter(string(content))
		if err != nil {
			logger.Error("Error parsing frontmatter", "error", err)
			doc = DocumentData{Content: string(content), Frontmatter: make(map[string]interface{})}
		}

		html := markdownToHTMLWithContext(cfg, doc.Content, cfg.Source)
		fmt.Fprint(out, html)
		return nil
	}

	// Neither -html nor -http provided and no source, show usage
	if fs := NewFlagSet("md2html"); fs != nil {
		fs.Usage()
	}
	return flag.ErrHelp
}

func runServer(ctx context.Context, cfg Config, logger *slog.Logger) error {
	s := newServer(cfg, logger)
	return s.Run(ctx)
}

func formatServerURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	if !strings.Contains(addr, "://") {
		return "http://" + addr
	}
	return addr
}

func generateDirectoryListing(cfg Config, dir string) (string, error) {
	files, err := findMarkdownFiles(dir, cfg.Depth)
	if err != nil {
		return "", err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})

	var buf strings.Builder

	if content, err := os.ReadFile(filepath.Join(dir, "index.md")); err == nil {
		buf.WriteString(string(content) + "\n\n---\n\n")
	}

	buf.WriteString(fmt.Sprintf("# Directory Listing: %s\n\n", dir))

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

func findMarkdownFiles(rootDir string, maxDepth int) ([]markdownFile, error) {
	var files []markdownFile
	seen := make(map[string]bool) // track real paths to avoid symlink cycles

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

			// Follow symlinks
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
				real, err := filepath.EvalSymlinks(apparentPath)
				if err != nil {
					real = realPath
				}
				if seen[real] {
					continue // avoid cycles
				}
				seen[real] = true
				if err := walkDir(realPath, apparentPath); err != nil {
					return err
				}
				continue
			}

			if depth >= maxDepth {
				continue
			}

			name := strings.ToLower(info.Name())
			if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown") {
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

func openBrowser(url string) bool {
	// Skip browser opening if environment variable is set (useful for tests)
	if os.Getenv("MD2HTML_NO_BROWSER") != "" {
		return true
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return false
	}
	return cmd.Start() == nil
}

func renderDocument(cfg Config, doc DocumentData, title, customCSS, filePath string) string {
	content := doc.Content
	if cfg.RenderFrontmatter && len(doc.Frontmatter) > 0 {
		if frontmatterYAML, err := yaml.Marshal(doc.Frontmatter); err == nil {
			content = "```yaml\n" + string(frontmatterYAML) + "```\n\n" + content
		}
	}

	html := markdownToHTMLWithContext(cfg, content, filePath)
	opts := RenderOptions{
		FilePath:    filePath,
		Description: llmsSummary(doc),
		EditURL:     editURL(cfg.EditURL, filePath),
	}
	return renderTemplateWithOptions(cfg, html, title, customCSS, true, doc.Frontmatter, opts)
}

func loadAllTemplates(cfg Config) (*template.Template, error) {
	tmpl, err := template.New("root").Funcs(template.FuncMap{
		"default": func(def, val interface{}) interface{} {
			if val == nil {
				return def
			}
			if s, ok := val.(string); ok && s == "" {
				return def
			}
			return val
		},
		"loadJSON": func(filename string) interface{} {
			data, err := loadJSONFile(filename)
			if err != nil {
				log.Printf("Error loading JSON %s: %v", filename, err)
				return nil
			}
			return data
		},
		"replace": strings.ReplaceAll,
		// dict creates a map from key-value pairs for passing to templates
		"dict": func(values ...interface{}) map[string]interface{} {
			if len(values)%2 != 0 {
				return nil
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					continue
				}
				dict[key] = values[i+1]
			}
			return dict
		},
		"navHref": func(currentFile, targetFile, htmlExt, indexFile string) string {
			return relativeRenderedLink(currentFile, targetFile, htmlExt, indexFile)
		},
		"asset": func(assets map[string]string, name string) string {
			if assets != nil {
				if v := assets[name]; v != "" {
					return v
				}
			}
			return name
		},
	}).ParseFS(templates, "templates/*.html", "templates/*/*.html")

	if err != nil {
		log.Printf("Error parsing embedded templates: %v", err)
		tmpl = template.New("root")
	}

	if cfg.TemplateDir != "" {
		for _, pattern := range []string{"*.html", "*/*.html"} {
			if t, err := tmpl.ParseGlob(filepath.Join(cfg.TemplateDir, pattern)); err == nil {
				tmpl = t
			}
		}
	}

	return tmpl, nil
}

// RenderOptions contains optional parameters for rendering.
type RenderOptions struct {
	Nav         *NavContext
	SiteTitle   string
	Data        interface{} // from -data-json
	FilePath    string      // source file path (for edit links)
	Version     string      // currently rendered version, when versioning is enabled
	Versions    []GitVersion
	RawMDURL    string
	Description string
	EditURL     string
	LastUpdated string
	Assets      map[string]string
}

func firstFrontmatterString(frontmatter map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := frontmatter[key]
		if !ok {
			continue
		}
		s, ok := value.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

func resolveMermaidThemes(frontmatter map[string]interface{}) (theme, darkTheme string, auto bool) {
	theme = "default"
	darkTheme = "dark"
	auto = true

	userTheme := firstFrontmatterString(frontmatter,
		"mermaid_theme",
		"mermaid-theme",
		"mermaidTheme",
	)
	userDarkTheme := firstFrontmatterString(frontmatter,
		"mermaid_dark_theme",
		"mermaid-dark-theme",
		"mermaidDarkTheme",
		"mermaid_theme_dark",
		"mermaid-theme-dark",
		"mermaidThemeDark",
	)

	if userTheme != "" {
		if strings.EqualFold(userTheme, "auto") {
			auto = true
		} else {
			theme = userTheme
			darkTheme = userTheme
			auto = false
		}
	}

	if userDarkTheme != "" {
		darkTheme = userDarkTheme
		auto = true
	}

	return theme, darkTheme, auto
}

func renderTemplate(cfg Config, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]interface{}) string {
	return renderTemplateWithOptions(cfg, htmlContent, title, customCSS, liveReload, frontmatter, RenderOptions{})
}

func renderTemplateWithOptions(cfg Config, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]interface{}, opts RenderOptions) string {
	tmpl, err := loadAllTemplates(cfg)
	if err != nil {
		log.Printf("Error loading templates: %v", err)
		return fmt.Sprintf("<p>Template loading error: %v</p>", err)
	}

	// Choose template based on whether we have navigation
	name := "layout"
	if opts.Nav != nil && opts.Nav.HasNav {
		if tmpl.Lookup("docs-layout") != nil {
			name = "docs-layout"
		} else {
			log.Printf("Warning: SUMMARY.md navigation loaded but docs-layout template not found, falling back to layout")
		}
	}
	if tmpl.Lookup(name) == nil {
		for _, n := range []string{"docs.html", "page.html", "live-reload.html", "base"} {
			if tmpl.Lookup(n) != nil {
				name = n
				break
			}
		}
	}

	if tmpl.Lookup(name) == nil {
		return "<p>No template found. Expected 'layout' template"
	}

	var buf bytes.Buffer
	mermaidTheme, mermaidDarkTheme, mermaidAutoTheme := resolveMermaidThemes(frontmatter)
	meta := pageMetadata(cfg, title, frontmatter, opts)

	data := struct {
		Title            string
		Content          template.HTML
		CustomCSS        template.CSS
		ChromaCSS        template.CSS
		Verbose          bool
		LiveReload       bool
		HTMLExt          string
		Frontmatter      map[string]interface{}
		Version          string
		Versions         []GitVersion
		Search           bool
		Nav              *NavContext
		SiteTitle        string
		Data             interface{}
		IndexFile        string
		MermaidTheme     string
		MermaidDarkTheme string
		MermaidAutoTheme bool
		FilePath         string
		AssetBase        string
		RawMDURL         string
		Description      string
		CanonicalURL     string
		OpenGraphImage   string
		LastUpdated      string
		EditURL          string
		Assets           map[string]string
		JSONSpec         template.JS
	}{
		Title:            title,
		Content:          template.HTML(htmlContent),
		CustomCSS:        template.CSS(customCSS),
		ChromaCSS:        template.CSS(generateChromaCSS()),
		Verbose:          cfg.Verbose,
		LiveReload:       liveReload,
		HTMLExt:          cfg.HTMLExt,
		Frontmatter:      frontmatter,
		Version:          opts.Version,
		Versions:         opts.Versions,
		Search:           cfg.Search,
		Nav:              opts.Nav,
		SiteTitle:        opts.SiteTitle,
		Data:             opts.Data,
		IndexFile:        cfg.Index,
		MermaidTheme:     mermaidTheme,
		MermaidDarkTheme: mermaidDarkTheme,
		MermaidAutoTheme: mermaidAutoTheme,
		FilePath:         opts.FilePath,
		AssetBase:        assetBase(opts.FilePath),
		RawMDURL:         opts.RawMDURL,
		Description:      meta.Description,
		CanonicalURL:     meta.CanonicalURL,
		OpenGraphImage:   meta.OpenGraphImage,
		LastUpdated:      meta.LastUpdated,
		EditURL:          opts.EditURL,
		Assets:           opts.Assets,
		JSONSpec:         jsonSpecBundleJSON(cfg),
	}

	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("Error executing template: %v", err)
		return fmt.Sprintf("<p>Template execution error: %v</p>", err)
	}
	return buf.String()
}

type renderMetadata struct {
	Description    string
	CanonicalURL   string
	OpenGraphImage string
	LastUpdated    string
}

func pageMetadata(cfg Config, title string, frontmatter map[string]interface{}, opts RenderOptions) renderMetadata {
	desc := firstFrontmatterString(frontmatter, "description")
	if desc == "" {
		desc = opts.Description
	}
	meta := renderMetadata{
		Description:    desc,
		OpenGraphImage: firstFrontmatterString(frontmatter, "og_image", "image"),
		LastUpdated:    opts.LastUpdated,
	}
	if cfg.SiteURL != "" && opts.FilePath != "" {
		meta.CanonicalURL = joinSiteURL(cfg.SiteURL, renderedPathForSource(opts.FilePath, cfg.HTMLExt, cfg.Index))
	}
	return meta
}

func joinSiteURL(base, pagePath string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	pagePath = strings.TrimLeft(filepath.ToSlash(pagePath), "/")
	if base == "" || pagePath == "" {
		return base
	}
	return base + "/" + pagePath
}

func editURL(pattern, filePath string) string {
	if pattern == "" || filePath == "" {
		return ""
	}
	return strings.ReplaceAll(pattern, "{path}", filepath.ToSlash(filePath))
}

func generateStaticHTML(ctx context.Context, cfg Config, logger *slog.Logger) error {
	sourceDir := cfg.Source
	if sourceDir == "" {
		sourceDir = "."
	}

	outputDir := cfg.HTML

	logger.Info("Generating static HTML", "source", sourceDir, "output", outputDir)

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Load JSON data if provided
	var jsonData interface{}
	if cfg.DataJSON != "" {
		var err error
		jsonData, err = loadJSONFile(cfg.DataJSON)
		if err != nil {
			logger.Error("Error loading JSON data file", "error", err, "file", cfg.DataJSON)
		} else {
			logger.Debug("Loaded JSON data", "file", cfg.DataJSON)
		}
	}

	// Load CSS if provided
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

	// Find all markdown files
	files, err := findMarkdownFiles(sourceDir, 100) // Use high depth for static generation
	if err != nil {
		return fmt.Errorf("failed to find markdown files: %v", err)
	}

	logger.Info("Found markdown files to process", "count", len(files))

	// Load navigation from SUMMARY.md or build it from the markdown tree.
	htmlExt := ""
	if cfg.HTMLExt != "" {
		htmlExt = "." + cfg.HTMLExt
	}
	nav, err := LoadNavigationOrAutoFromDir(sourceDir, htmlExt)
	if err != nil {
		logger.Error("Error loading navigation", "error", err)
	} else if nav != nil && len(nav.Items) > 0 {
		logger.Info("Loaded navigation", "pages", len(nav.Flat))
	}
	lastUpdated := map[string]string{}
	if times, err := gitLastUpdated(sourceDir, files); err == nil {
		lastUpdated = times
	} else if cfg.Verbose {
		logger.Debug("git metadata unavailable", "error", err)
	}
	assets := map[string]string{}
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

	// Process each markdown file
	for _, file := range files {
		// Check for draft frontmatter and skip
		if isDraft(filepath.Join(sourceDir, file.RelPath)) {
			logger.Debug("Skipping draft", "file", file.RelPath)
			continue
		}
		opts := RenderOptions{
			SiteTitle:   cfg.Title,
			Data:        jsonData,
			FilePath:    file.RelPath,
			EditURL:     editURL(cfg.EditURL, file.RelPath),
			LastUpdated: lastUpdated[filepath.ToSlash(file.RelPath)],
			Assets:      assets,
		}
		if cfg.LLMS {
			opts.RawMDURL = rawMarkdownURL(file.RelPath)
		}
		if nav != nil {
			opts.Nav = nav.ForPage(file.RelPath)
		}
		if err := processMarkdownFileWithOpts(file, sourceDir, outputDir, cssContent, cfg, opts); err != nil {
			logger.Error("Error processing file", "error", err, "file", file.RelPath)
			continue
		}
		logger.Debug("Generated file", "file", file.RelPath)
	}

	// Handle index file if specified
	if cfg.Index != "" {
		indexFile := filepath.Join(sourceDir, cfg.Index)
		if _, err := os.Stat(indexFile); err == nil {
			indexOpts := RenderOptions{
				SiteTitle:   cfg.Title,
				Data:        jsonData,
				FilePath:    cfg.Index,
				EditURL:     editURL(cfg.EditURL, cfg.Index),
				LastUpdated: lastUpdated[filepath.ToSlash(cfg.Index)],
				Assets:      assets,
			}
			if cfg.LLMS {
				indexOpts.RawMDURL = rawMarkdownURL(cfg.Index)
			}
			if nav != nil {
				indexOpts.Nav = nav.ForPage(cfg.Index)
			}
			if err := processIndexFileWithOpts(indexFile, outputDir, cssContent, cfg, indexOpts); err != nil {
				logger.Error("Error processing index file", "error", err, "file", indexFile)
			} else {
				logger.Debug("Processed index file", "file", indexFile)
			}
		}
	} else {
		// Generate table of contents as index.html
		if err := generateTOCIndex(sourceDir, outputDir, files, cssContent, cfg, assets); err != nil {
			logger.Error("Error generating TOC index", "error", err)
		} else {
			logger.Debug("Generated TOC index")
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

func processMarkdownFile(file markdownFile, sourceDir, outputDir, cssContent string, cfg Config) error {
	return processMarkdownFileWithNav(file, sourceDir, outputDir, cssContent, cfg, nil)
}

func processMarkdownFileWithOpts(file markdownFile, sourceDir, outputDir, cssContent string, cfg Config, opts RenderOptions) error {
	sourcePath := filepath.Join(sourceDir, file.RelPath)

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	doc, err := parseFrontmatter(string(content))
	if err != nil {
		log.Printf("Error parsing frontmatter in %s: %v", file.RelPath, err)
		doc = DocumentData{Content: string(content), Frontmatter: make(map[string]interface{})}
	}

	htmlContent := markdownToHTMLWithContext(cfg, doc.Content, file.RelPath)
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

	title := cfg.Title
	if docTitle, ok := doc.Frontmatter["title"].(string); ok && docTitle != "" {
		title = docTitle
	} else {
		title = strings.TrimSuffix(filepath.Base(file.RelPath), filepath.Ext(file.RelPath))
	}

	finalHTML := renderTemplateWithOptions(cfg, htmlContent, title, cssContent, false, doc.Frontmatter, opts)
	return os.WriteFile(outputPath, []byte(finalHTML), 0644)
}

func processMarkdownFileWithNav(file markdownFile, sourceDir, outputDir, cssContent string, cfg Config, nav *Navigation) error {
	sourcePath := filepath.Join(sourceDir, file.RelPath)

	// Read and parse the markdown file
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	doc, err := parseFrontmatter(string(content))
	if err != nil {
		log.Printf("Error parsing frontmatter in %s: %v", file.RelPath, err)
		doc = DocumentData{Content: string(content), Frontmatter: make(map[string]interface{})}
	}

	// Generate HTML content
	htmlContent := markdownToHTMLWithContext(cfg, doc.Content, file.RelPath)

	// Determine output file path
	baseName := strings.TrimSuffix(file.RelPath, filepath.Ext(file.RelPath))
	outputPath := baseName
	// When HTMLExt is empty, only index gets .html extension
	// When HTMLExt is set, all files get that extension
	if cfg.HTMLExt != "" {
		outputPath = baseName + "." + cfg.HTMLExt
	}
	outputPath = filepath.Join(outputDir, outputPath)

	// Create output directory if needed
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	// Render with template
	title := cfg.Title
	if docTitle, ok := doc.Frontmatter["title"].(string); ok && docTitle != "" {
		title = docTitle
	} else {
		title = strings.TrimSuffix(filepath.Base(file.RelPath), filepath.Ext(file.RelPath))
	}

	// Get navigation context for this page
	var navCtx *NavContext
	if nav != nil {
		navCtx = nav.ForPage(file.RelPath)
	}

	opts := RenderOptions{
		Nav:         navCtx,
		SiteTitle:   cfg.Title,
		FilePath:    file.RelPath,
		Description: llmsSummary(doc),
		EditURL:     editURL(cfg.EditURL, file.RelPath),
	}

	finalHTML := renderTemplateWithOptions(cfg, htmlContent, title, cssContent, false, doc.Frontmatter, opts)

	// Write output file
	return os.WriteFile(outputPath, []byte(finalHTML), 0644)
}

func processIndexFileWithOpts(indexPath, outputDir, cssContent string, cfg Config, opts RenderOptions) error {
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	doc, err := parseFrontmatter(string(content))
	if err != nil {
		log.Printf("Error parsing frontmatter in index file: %v", err)
		doc = DocumentData{Content: string(content), Frontmatter: make(map[string]interface{})}
	}

	htmlPath := opts.FilePath
	if htmlPath == "" {
		htmlPath = filepath.Base(indexPath)
	}
	htmlContent := markdownToHTMLWithContext(cfg, doc.Content, htmlPath)
	opts.Description = llmsSummary(doc)

	title := cfg.Title
	if docTitle, ok := doc.Frontmatter["title"].(string); ok && docTitle != "" {
		title = docTitle
	}

	finalHTML := renderTemplateWithOptions(cfg, htmlContent, title, cssContent, false, doc.Frontmatter, opts)

	indexOutputPath := filepath.Join(outputDir, "index.html")
	if cfg.LLMS && opts.FilePath != "" {
		if err := copyRawMarkdown(indexPath, outputDir, opts.FilePath); err != nil {
			return err
		}
	}
	return os.WriteFile(indexOutputPath, []byte(finalHTML), 0644)
}

func processIndexFile(indexPath, outputDir, cssContent string, cfg Config) error {
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	doc, err := parseFrontmatter(string(content))
	if err != nil {
		log.Printf("Error parsing frontmatter in index file: %v", err)
		doc = DocumentData{Content: string(content), Frontmatter: make(map[string]interface{})}
	}

	htmlContent := markdownToHTMLWithContext(cfg, doc.Content, filepath.Base(indexPath))

	title := cfg.Title
	if docTitle, ok := doc.Frontmatter["title"].(string); ok && docTitle != "" {
		title = docTitle
	}

	finalHTML := renderTemplate(cfg, htmlContent, title, cssContent, false, doc.Frontmatter)

	indexOutputPath := filepath.Join(outputDir, "index.html")
	return os.WriteFile(indexOutputPath, []byte(finalHTML), 0644)
}

func generateTOCIndex(sourceDir, outputDir string, files []markdownFile, cssContent string, cfg Config, assets map[string]string) error {
	// Generate table of contents markdown
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("# Directory Listing: %s\n\n", sourceDir))

	if len(files) == 0 {
		buf.WriteString("*No markdown files found.*\n")
	} else {
		for _, f := range files {
			url := renderedPathForSource(f.RelPath, cfg.HTMLExt, cfg.Index)
			buf.WriteString(fmt.Sprintf("- [%s](%s) (%d bytes, %s)\n",
				f.RelPath, url, f.Size, f.ModTime.Format("2006-01-02 15:04")))
		}
	}

	// Convert to HTML
	htmlContent := markdownToHTMLWithContext(cfg, buf.String(), "")

	// Render with template
	doc := DocumentData{Content: buf.String(), Frontmatter: make(map[string]interface{})}
	finalHTML := renderTemplateWithOptions(cfg, htmlContent, "Directory Listing", cssContent, false, doc.Frontmatter, RenderOptions{Assets: assets})

	// Write index.html
	indexOutputPath := filepath.Join(outputDir, "index.html")
	return os.WriteFile(indexOutputPath, []byte(finalHTML), 0644)
}
