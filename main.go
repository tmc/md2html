package md2html

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tmc/md2html/internal/markdown/components"
	"github.com/tmc/md2html/internal/markdown/jsonspec"
)

type Config struct {
	// Chdir is a directory to change to before anything else, like
	// go -C or make -C. Every relative path — the source, output,
	// css, and configuration files — then resolves against it.
	Chdir             string
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
	// OGImage is the social card image used by pages whose frontmatter
	// names none. A relative value resolves against SiteURL, so a site
	// served from a subdirectory still advertises an absolute URL.
	OGImage string
	EditURL string
	Nav     bool
	Watch   string
	// Base is the URL path prefix the served tree is published under,
	// such as "/docs". It lets root-absolute links written for the
	// published site resolve when previewing a subtree. Server mode only.
	Base string
	// Drafts renders pages whose frontmatter sets draft: true instead of
	// skipping them. Drafts stay excluded from llms.txt either way;
	// templates can check .Frontmatter.draft to mark rendered drafts.
	Drafts bool
	// Format selects an opt-in structured Markdown presentation profile.
	// The empty string uses ordinary Markdown behavior. "okf" enables
	// Open Knowledge Format link handling.
	Format string

	// Vet enables non-blocking mdvet checks. When true, [Run] reports
	// any diagnostics it finds to the logger (or stderr) at warn level
	// before continuing. Diagnostics never cause Run to fail.
	Vet bool
	// VetChecks is a comma-separated list of mdvet check names to run
	// when Vet is true. Empty means run every check. Unknown names are
	// logged and skipped — vet must never block rendering.
	VetChecks string

	// JSONSpec is a directory containing jsonspec.json and any
	// *.schema.json files used for JSON discriminator enrichment.
	JSONSpec string

	// Components is a directory containing components.json and the
	// html/template files it names, defining layout components in
	// addition to the built-in ones.
	Components string

	// Stars fetches the star count of the repository named in the
	// navigation source and shows it beside the repository link. It is
	// the only thing here that needs the network, so it is opt-in.
	Stars bool

	// Icons is a directory of .svg files, one per icon name, drawn on
	// by pages that name an icon in their frontmatter. It replaces the
	// icon library selected by docs.json.
	Icons string
	// NoIcons disables both built-in and directory icon sets.
	NoIcons bool

	componentRegistry components.Registry
	iconSet           map[string]template.HTML
	iconStyles        map[string]map[string]template.HTML
	iconAttribution   string
	iconMissing       *sync.Map
	iconLogger        *slog.Logger
	iconDisabled      bool
	// starsAPI overrides the host star counts are read from. Only tests
	// set it; the empty value means the real API.
	starsAPI string

	jsonSpecConfig jsonspec.Config
	jsonSpecBundle template.JS
	jsonSpecReady  bool
}

// NewFlagSet returns a FlagSet configured for the md2html CLI.
func NewFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.String("C", "", "change to directory before doing anything else")
	fs.String("http", "", "HTTP server bind address")
	fs.String("html", "", "output directory for static HTML generation (disables server mode)")
	fs.Bool("open", false, "automatically open browser")
	fs.Bool("v", false, "verbose logging")
	fs.String("title", defaultTitle, "HTML title")
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
	fs.String("og-image", "", "social card image for pages that name none in frontmatter; relative values resolve against -site-url")
	fs.String("edit-url", "", "URL template for edit links; {path} is replaced with the source path")
	fs.Bool("nav", false, "render docs navigation from SUMMARY.md or the markdown tree")
	fs.String("watch", "auto", "live reload file watching: auto, true, or false")
	fs.String("base", "", "URL path prefix the tree is served under (e.g. /docs), for previewing a subtree of a published site")
	fs.Bool("drafts", false, "render pages marked draft: true instead of skipping them")
	fs.String("format", "", "structured Markdown format: okf")
	fs.String("jsonspec", "", "directory containing jsonspec.json and *.schema.json files")
	fs.String("components", "", "directory containing components.json and component templates")
	fs.String("icons", "", "directory of .svg files named for the icons pages request in frontmatter (replaces the library selected by docs.json)")
	fs.Bool("no-icons", false, "disable navigation and component icons")
	fs.Bool("github-stars", false, "fetch the star count of the repository named in docs.json and show it in the bar")
	fs.Bool("vet", false, "run mdvet checks on source markdown and report diagnostics (does not block rendering)")
	fs.String("vet-checks", "", "comma-separated mdvet check names to run with -vet (default: all)")
	return fs
}

// ConfigFromFlags creates a Config from an initialized FlagSet.
func ConfigFromFlags(fs *flag.FlagSet) Config {
	return Config{
		Chdir:             flagString(fs, "C"),
		HTTP:              flagString(fs, "http"),
		HTML:              flagString(fs, "html"),
		Open:              flagBool(fs, "open"),
		Verbose:           flagBool(fs, "v"),
		Title:             flagString(fs, "title"),
		CSS:               flagString(fs, "css"),
		Depth:             flagInt(fs, "depth"),
		TOC:               flagBool(fs, "toc"),
		AllowUnsafe:       flagBool(fs, "allow-unsafe"),
		TemplateDir:       flagString(fs, "templates"),
		DataJSON:          flagString(fs, "data-json"),
		RenderFrontmatter: flagBool(fs, "render-frontmatter"),
		Index:             flagString(fs, "index"),
		HTMLExt:           flagString(fs, "html-ext"),
		Versions:          flagBool(fs, "versions"),
		VersionPattern:    flagString(fs, "version-pattern"),
		VersionBranches:   flagBool(fs, "version-branches"),
		VersionDefault:    flagString(fs, "version-default"),
		Search:            flagBool(fs, "search"),
		LLMS:              flagBool(fs, "llms"),
		SiteURL:           flagString(fs, "site-url"),
		OGImage:           flagString(fs, "og-image"),
		EditURL:           flagString(fs, "edit-url"),
		Nav:               flagBool(fs, "nav"),
		Watch:             flagString(fs, "watch"),
		Base:              flagString(fs, "base"),
		Drafts:            flagBool(fs, "drafts"),
		Format:            flagString(fs, "format"),
		JSONSpec:          flagString(fs, "jsonspec"),
		Components:        flagString(fs, "components"),
		Icons:             flagString(fs, "icons"),
		NoIcons:           flagBool(fs, "no-icons"),
		Stars:             flagBool(fs, "github-stars"),
		Vet:               flagBool(fs, "vet"),
		VetChecks:         flagString(fs, "vet-checks"),
	}
}

func flagString(fs *flag.FlagSet, name string) string {
	if f := fs.Lookup(name); f != nil {
		return f.Value.String()
	}
	return ""
}

func flagBool(fs *flag.FlagSet, name string) bool {
	f := fs.Lookup(name)
	return f != nil && f.Value.String() == "true"
}

func flagInt(fs *flag.FlagSet, name string) int {
	f := fs.Lookup(name)
	if f == nil {
		return 0
	}
	getter, ok := f.Value.(flag.Getter)
	if !ok {
		return 0
	}
	value, _ := getter.Get().(int)
	return value
}

func Run(ctx context.Context, cfg Config, logger *slog.Logger, out io.Writer, args []string) error {
	if logger == nil {
		logger = slog.Default()
	}

	// Set up signal handling
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Change directory first so every later relative path resolves
	// against it. This is process-global, which is what -C means.
	if cfg.Chdir != "" {
		if err := os.Chdir(cfg.Chdir); err != nil {
			return fmt.Errorf("chdir: %w", err)
		}
	}

	if err := validateFormat(cfg.Format); err != nil {
		return err
	}
	var err error
	cfg, err = prepareJSONSpec(cfg, logger)
	if err != nil {
		return err
	}
	cfg, err = prepareComponents(cfg)
	if err != nil {
		return err
	}

	// Handle positional arguments
	if len(args) > 0 {
		cfg.Source = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("too many positional arguments")
	}

	// Icons are looked for beside the documentation when no directory
	// was named, so the source has to be known first.
	cfg, err = prepareIcons(cfg)
	if err != nil {
		return err
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
	cfg.iconLogger = logger

	// Run mdvet checks before any rendering. Diagnostics are reported
	// to the logger but never cause Run to fail.
	if cfg.Vet {
		runVet(cfg, logger)
	}

	// TODO: clean up handling stdin and choosing between modes

	// If -html flag is provided, generate static HTML
	if cfg.HTML != "" {
		if cfg.Base != "" {
			return fmt.Errorf("-base applies to server mode; static output is served at whatever prefix the host uses")
		}
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
			doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
		}

		html, err := markdownToHTMLWithContext(cfg, promoteTitleHeading(doc), cfg.Source)
		if err != nil {
			return err
		}
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
	if _, err := watchEnabled(cfg.Watch, cfg.Source); err != nil {
		return err
	}
	s := newServer(ctx, cfg, logger)
	return s.Run(ctx)
}

func watchEnabled(mode, source string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return source != "-", nil
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid watch mode %q (want auto, true, or false)", mode)
	}
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

func findMarkdownFiles(rootDir string, maxDepth int) ([]markdownFile, error) {
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

func openBrowser(ctx context.Context, url string) bool {
	// Skip browser opening if environment variable is set (useful for tests)
	if os.Getenv("MD2HTML_NO_BROWSER") != "" {
		return true
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", url)
	default:
		return false
	}
	return cmd.Start() == nil
}

func loadAllTemplates(cfg Config) (*template.Template, error) {
	tmpl, err := template.New("root").Funcs(template.FuncMap{
		"default": func(def, val any) any {
			if val == nil {
				return def
			}
			if s, ok := val.(string); ok && s == "" {
				return def
			}
			return val
		},
		"loadJSON": func(filename string) any {
			data, err := loadJSONFile(filename)
			if err != nil {
				slog.Default().Error("Error loading JSON", "file", filename, "error", err)
				return nil
			}
			return data
		},
		"replace": strings.ReplaceAll,
		// dict creates a map from key-value pairs for passing to templates
		"dict": func(values ...any) map[string]any {
			if len(values)%2 != 0 {
				return nil
			}
			dict := make(map[string]any, len(values)/2)
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
		"navIcon": cfg.navIconType,
		"asset": func(assets map[string]string, name string) string {
			if assets != nil {
				if v := assets[name]; v != "" {
					return v
				}
			}
			return name
		},
		"jsonSpec": func() template.JS {
			return cfg.jsonSpecBundle
		},
	}).ParseFS(templates, "templates/*.html", "templates/*/*.html")

	if err != nil {
		slog.Default().Error("Error parsing embedded templates", "error", err)
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

// RenderOptions contains optional data passed to rendering and templates.
//
// Its exported fields are part of the template compatibility contract and
// follow semantic versioning.
type RenderOptions struct {
	// Nav is the navigation context for the current page.
	Nav *NavContext
	// SiteTitle is the configured site title.
	SiteTitle string
	// Data is the decoded value loaded from -data-json.
	Data any
	// FilePath is the source Markdown path relative to the rendered tree.
	FilePath string
	// Version is the currently rendered git version, when versioning is enabled.
	Version string
	// Versions is the list of available git-backed documentation versions.
	Versions []GitVersion
	// RawMDURL is the URL for the source Markdown file, when available.
	RawMDURL string
	// Description is the page description used for metadata and search.
	Description string
	// EditURL is the resolved edit link for the current page.
	EditURL string
	// LastUpdated is the page's last modification date, when known.
	LastUpdated string
	// Assets maps logical asset names to emitted, fingerprinted paths.
	Assets map[string]string
	// Accent and AccentDark are the site's brand color for light and dark
	// rendering, as CSS hex colors. Empty leaves the built-in accent.
	Accent     string
	AccentDark string
	// Repo is the "owner/name" of the documented source repository and
	// RepoURL its address. Empty renders no repository link.
	Repo    string
	RepoURL string
	// NavLinks are plain links shown in the navigation bar.
	NavLinks []SiteLink
	// Stars is the repository's star count, already formatted. Empty
	// shows the link without a count.
	Stars string
	// ShowStars reports that a count was asked for. A deployed page
	// refreshes the count in the browser, so the element has to be
	// rendered even when the build could not fetch one.
	ShowStars bool
}

func firstFrontmatterString(frontmatter map[string]any, keys ...string) string {
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

func resolveMermaidThemes(frontmatter map[string]any) (theme, darkTheme string, auto bool) {
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

func renderTemplate(cfg Config, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]any) (string, error) {
	return renderTemplateWithOptions(cfg, htmlContent, title, customCSS, liveReload, frontmatter, RenderOptions{})
}

func renderTemplateWithOptions(cfg Config, htmlContent, title, customCSS string, liveReload bool, frontmatter map[string]any, opts RenderOptions) (string, error) {
	tmpl, err := loadAllTemplates(cfg)
	if err != nil {
		return "", fmt.Errorf("load templates: %w", err)
	}

	// Choose template based on whether we have navigation
	name := "layout"
	if opts.Nav != nil && opts.Nav.HasNav {
		if tmpl.Lookup("docs-layout") != nil {
			name = "docs-layout"
		} else {
			slog.Default().Warn("SUMMARY.md navigation loaded but docs-layout template not found, falling back to layout")
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
		return "", fmt.Errorf("no template found: expected layout template")
	}

	var buf bytes.Buffer
	mermaidTheme, mermaidDarkTheme, mermaidAutoTheme := resolveMermaidThemes(frontmatter)
	meta := pageMetadata(cfg, title, frontmatter, opts)
	var iconAttribution template.HTML
	if cfg.iconAttribution != "" {
		iconAttribution = template.HTML("<!-- " + cfg.iconAttribution + " -->")
	}

	data := templateData{
		Title:             title,
		Content:           template.HTML(htmlContent),
		CustomCSS:         template.CSS(customCSS),
		ChromaCSS:         template.CSS(generateChromaCSS()),
		Verbose:           cfg.Verbose,
		LiveReload:        liveReload,
		HTMLExt:           cfg.HTMLExt,
		Frontmatter:       frontmatter,
		Version:           opts.Version,
		Versions:          opts.Versions,
		Search:            cfg.Search,
		Nav:               opts.Nav,
		SiteTitle:         opts.SiteTitle,
		Data:              opts.Data,
		IndexFile:         cfg.Index,
		MermaidTheme:      mermaidTheme,
		MermaidDarkTheme:  mermaidDarkTheme,
		MermaidAutoTheme:  mermaidAutoTheme,
		FilePath:          opts.FilePath,
		AssetBase:         assetBase(opts.FilePath),
		RawMDURL:          opts.RawMDURL,
		HasMath:           pageHasMath(htmlContent),
		Description:       meta.Description,
		CanonicalURL:      meta.CanonicalURL,
		OpenGraphImage:    meta.OpenGraphImage,
		OpenGraphImageAlt: meta.OpenGraphImageAlt,
		OpenGraphType:     meta.OpenGraphType,
		LastUpdated:       meta.LastUpdated,
		EditURL:           opts.EditURL,
		Assets:            opts.Assets,
		Accent:            template.CSS(opts.Accent),
		AccentDark:        template.CSS(opts.AccentDark),
		Repo:              opts.Repo,
		RepoURL:           opts.RepoURL,
		NavLinks:          opts.NavLinks,
		Stars:             opts.Stars,
		ShowStars:         opts.ShowStars,
		IconAttribution:   iconAttribution,
	}

	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", name, err)
	}
	return buf.String(), nil
}

type templateData struct {
	Title            string
	Content          template.HTML
	CustomCSS        template.CSS
	ChromaCSS        template.CSS
	Verbose          bool
	LiveReload       bool
	HTMLExt          string
	Frontmatter      map[string]any
	Version          string
	Versions         []GitVersion
	Search           bool
	Nav              *NavContext
	SiteTitle        string
	Data             any
	IndexFile        string
	MermaidTheme     string
	MermaidDarkTheme string
	MermaidAutoTheme bool
	FilePath         string
	AssetBase        string
	RawMDURL         string
	// HasMath reports whether the page content contains TeX math
	// delimiters outside code regions, so templates can load MathJax
	// only where it is needed.
	HasMath      bool
	Description  string
	CanonicalURL string
	// OpenGraphImage is the absolute URL of the page's social card
	// image, OpenGraphImageAlt its description, and OpenGraphType the
	// og:type the page claims: "website" for the site root, "article"
	// for every other page.
	OpenGraphImage    string
	OpenGraphImageAlt string
	OpenGraphType     string
	LastUpdated       string
	EditURL           string
	Assets            map[string]string
	// Accent and AccentDark are validated CSS colors, empty unless the
	// navigation source named one.
	Accent     template.CSS
	AccentDark template.CSS
	// Repo and RepoURL name the source repository shown in the bar, and
	// Stars its formatted star count when one was fetched. ShowStars
	// reports that a count was asked for, which is what decides whether
	// the page carries the element the browser refreshes.
	Repo      string
	RepoURL   string
	Stars     string
	ShowStars bool
	// IconAttribution credits the built-in icon set in one place instead
	// of repeating its license comment in every inlined SVG.
	IconAttribution template.HTML
	// NavLinks are plain links shown in the navigation bar.
	NavLinks []SiteLink
}

type renderMetadata struct {
	Description       string
	CanonicalURL      string
	OpenGraphImage    string
	OpenGraphImageAlt string
	OpenGraphType     string
	LastUpdated       string
}

func pageMetadata(cfg Config, title string, frontmatter map[string]any, opts RenderOptions) renderMetadata {
	desc := firstFrontmatterString(frontmatter, "description")
	if desc == "" {
		desc = opts.Description
	}
	meta := renderMetadata{
		Description:       desc,
		OpenGraphImage:    firstFrontmatterString(frontmatter, "og_image", "image"),
		OpenGraphImageAlt: firstFrontmatterString(frontmatter, "og_image_alt", "image_alt"),
		OpenGraphType:     "article",
		LastUpdated:       opts.LastUpdated,
	}
	if opts.FilePath != "" {
		rendered := renderedPathForSource(opts.FilePath, cfg.HTMLExt, cfg.Index)
		page := canonicalPagePath(rendered, cfg.HTMLExt)
		if page == "" {
			meta.OpenGraphType = "website"
		}
		if cfg.SiteURL != "" {
			meta.CanonicalURL = joinSiteURL(cfg.SiteURL, page)
		}
	}
	// A card image has to be an absolute URL: the crawler fetches it
	// without a document to resolve against. The page URL is the base,
	// so a page-relative name in frontmatter means what it says, while
	// the site-wide default is written relative to the site root.
	if meta.OpenGraphImage != "" {
		meta.OpenGraphImage = absoluteURL(meta.CanonicalURL, meta.OpenGraphImage)
	} else if cfg.OGImage != "" {
		base := strings.TrimRight(strings.TrimSpace(cfg.SiteURL), "/")
		if base != "" {
			base += "/"
		}
		meta.OpenGraphImage = absoluteURL(base, cfg.OGImage)
		meta.OpenGraphImageAlt = ""
	}
	return meta
}

// absoluteURL resolves ref against base. A ref that is already absolute
// is returned unchanged, and so is one that cannot be resolved because
// no base URL was configured: half a URL is no more useful than a
// relative one, and dropping it would hide the mistake.
func absoluteURL(base, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	if u.IsAbs() || strings.HasPrefix(ref, "//") {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil || !b.IsAbs() {
		return ref
	}
	return b.ResolveReference(u).String()
}

func joinSiteURL(base, pagePath string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	pagePath = strings.TrimLeft(filepath.ToSlash(pagePath), "/")
	if base == "" {
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
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Copy assets before rendering, so anything generated under the same
	// name — llms.txt, the search assets — is what survives.
	assetCount, err := copySourceAssets(sourceDir, outputDir, logger)
	if err != nil {
		logger.Error("Error copying assets", "error", err)
	}
	if assetCount > 0 {
		logger.Info("Copied assets", "count", assetCount)
	}

	// Load JSON data if provided
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

	var nav *Navigation
	var site siteInfo
	if cfg.Nav {
		// Load navigation from SUMMARY.md or build it from the markdown tree.
		htmlExt := ""
		if cfg.HTMLExt != "" {
			htmlExt = "." + cfg.HTMLExt
		}
		var err error
		nav, site, err = navigationForDir(sourceDir, htmlExt)
		if err != nil {
			logger.Error("Error loading navigation", "error", err)
		} else if nav != nil && len(nav.Items) > 0 {
			cfg.Title = siteTitle(cfg.Title, site.Name)
			logger.Info("Loaded navigation", "pages", len(nav.Flat))
			site.Stars = repoStars(ctx, cfg, site.Repo, logger)
		}
	}
	lastUpdated := map[string]string{}
	if times, err := gitLastUpdated(ctx, sourceDir, files); err == nil {
		lastUpdated = times
	} else if cfg.Verbose {
		logger.Debug("git metadata unavailable", "error", err)
	}
	assets := map[string]string{}
	if cfg.jsonSpecBundle != "" {
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

	// Process each markdown file
	var renderErrors []error
	for _, file := range files {
		// Check for draft frontmatter and skip unless drafts are requested
		if !cfg.Drafts && isDraft(filepath.Join(sourceDir, file.RelPath)) {
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
			Accent:      site.Accent,
			AccentDark:  site.AccentDark,
			Repo:        site.Repo,
			RepoURL:     site.RepoURL,
			NavLinks:    site.Links,
			Stars:       site.Stars,
			ShowStars:   cfg.Stars,
		}
		if cfg.LLMS {
			opts.RawMDURL = rawMarkdownURL(file.RelPath)
		}
		if nav != nil {
			opts.Nav = nav.ForPage(file.RelPath)
		}
		if err := processMarkdownFileWithOpts(file, sourceDir, outputDir, cssContent, cfg, opts); err != nil {
			logger.Error("Error processing file", "error", err, "file", file.RelPath)
			renderErrors = append(renderErrors, fmt.Errorf("process %s: %w", file.RelPath, err))
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
				Accent:      site.Accent,
				AccentDark:  site.AccentDark,
				Repo:        site.Repo,
				RepoURL:     site.RepoURL,
				NavLinks:    site.Links,
				Stars:       site.Stars,
				ShowStars:   cfg.Stars,
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
		if err := generateTOCIndex(outputDir, files, cssContent, cfg, assets); err != nil {
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

func processMarkdownFileWithOpts(file markdownFile, sourceDir, outputDir, cssContent string, cfg Config, opts RenderOptions) error {
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

	htmlContent, err := markdownToHTMLWithContext(cfg, promoteTitleHeading(doc), file.RelPath)
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

	finalHTML, err := renderTemplateWithOptions(cfg, htmlContent, title, cssContent, false, doc.Frontmatter, opts)
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

func processIndexFileWithOpts(indexPath, outputDir, cssContent string, cfg Config, opts RenderOptions) error {
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
	htmlContent, err := markdownToHTMLWithContext(cfg, promoteTitleHeading(doc), htmlPath)
	if err != nil {
		return err
	}
	opts.Description = llmsSummary(doc)

	title := cfg.Title
	if docTitle, ok := doc.Frontmatter["title"].(string); ok && docTitle != "" {
		title = docTitle
	}

	finalHTML, err := renderTemplateWithOptions(cfg, htmlContent, title, cssContent, false, doc.Frontmatter, opts)
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

func generateTOCIndex(outputDir string, files []markdownFile, cssContent string, cfg Config, assets map[string]string) error {
	// Generate table of contents markdown
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

	// Convert to HTML
	htmlContent, err := markdownToHTMLWithContext(cfg, buf.String(), "")
	if err != nil {
		return err
	}

	// Render with template
	doc := DocumentData{Content: buf.String(), Frontmatter: make(map[string]any)}
	finalHTML, err := renderTemplateWithOptions(cfg, htmlContent, listingTitle(""), cssContent, false, doc.Frontmatter, RenderOptions{Assets: assets, SiteTitle: cfg.Title})
	if err != nil {
		return err
	}

	// Write index.html
	indexOutputPath := filepath.Join(outputDir, "index.html")
	return os.WriteFile(indexOutputPath, []byte(finalHTML), 0644)
}
