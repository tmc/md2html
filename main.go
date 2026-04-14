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

	"gopkg.in/yaml.v2"
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

	// TODO: clean up handling stdin and choosing between modes

	// If -html flag is provided, generate static HTML
	if cfg.HTML != "" {
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

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path and depth
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		// Skip if we've exceeded max depth
		depth := strings.Count(relPath, string(filepath.Separator))
		if depth >= maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if it's a markdown file
		if !info.IsDir() {
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
	})

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
	return renderTemplate(cfg, html, title, customCSS, true, doc.Frontmatter)
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
	Nav       *NavContext
	SiteTitle string
	Data      interface{} // from -data-json
	FilePath  string      // source file path (for edit links)
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
	}{
		Title:            title,
		Content:          template.HTML(htmlContent),
		CustomCSS:        template.CSS(customCSS),
		ChromaCSS:        template.CSS(generateChromaCSS()),
		Verbose:          cfg.Verbose,
		LiveReload:       liveReload,
		HTMLExt:          cfg.HTMLExt,
		Frontmatter:      frontmatter,
		Version:          "",
		Versions:         nil,
		Search:           cfg.Search,
		Nav:              opts.Nav,
		SiteTitle:        opts.SiteTitle,
		Data:             opts.Data,
		IndexFile:        cfg.Index,
		MermaidTheme:     mermaidTheme,
		MermaidDarkTheme: mermaidDarkTheme,
		MermaidAutoTheme: mermaidAutoTheme,
		FilePath:         opts.FilePath,
	}

	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("Error executing template: %v", err)
		return fmt.Sprintf("<p>Template execution error: %v</p>", err)
	}
	return buf.String()
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

	// Load navigation from SUMMARY.md if present
	htmlExt := ""
	if cfg.HTMLExt != "" {
		htmlExt = "." + cfg.HTMLExt
	}
	nav := LoadNavigationFromDir(sourceDir, htmlExt)
	if nav != nil {
		logger.Info("Loaded navigation from SUMMARY.md", "pages", len(nav.Flat))
	}

	// Process each markdown file
	for _, file := range files {
		// Check for draft frontmatter and skip
		if isDraft(filepath.Join(sourceDir, file.RelPath)) {
			logger.Debug("Skipping draft", "file", file.RelPath)
			continue
		}
		opts := RenderOptions{SiteTitle: cfg.Title, Data: jsonData, FilePath: file.RelPath}
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
			indexOpts := RenderOptions{SiteTitle: cfg.Title, Data: jsonData, FilePath: cfg.Index}
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
		if err := generateTOCIndex(sourceDir, outputDir, files, cssContent, cfg); err != nil {
			logger.Error("Error generating TOC index", "error", err)
		} else {
			logger.Debug("Generated TOC index")
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

	baseName := strings.TrimSuffix(file.RelPath, filepath.Ext(file.RelPath))
	outputPath := baseName
	if cfg.HTMLExt != "" {
		outputPath = baseName + "." + cfg.HTMLExt
	}
	outputPath = filepath.Join(outputDir, outputPath)

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
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
		Nav:       navCtx,
		SiteTitle: cfg.Title,
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

	title := cfg.Title
	if docTitle, ok := doc.Frontmatter["title"].(string); ok && docTitle != "" {
		title = docTitle
	}

	finalHTML := renderTemplateWithOptions(cfg, htmlContent, title, cssContent, false, doc.Frontmatter, opts)

	indexOutputPath := filepath.Join(outputDir, "index.html")
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

func generateTOCIndex(sourceDir, outputDir string, files []markdownFile, cssContent string, cfg Config) error {
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
	finalHTML := renderTemplate(cfg, htmlContent, "Directory Listing", cssContent, false, doc.Frontmatter)

	// Write index.html
	indexOutputPath := filepath.Join(outputDir, "index.html")
	return os.WriteFile(indexOutputPath, []byte(finalHTML), 0644)
}
