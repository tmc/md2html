package md2html

import (
	"flag"
	"html/template"
	"log/slog"
	"sync"

	"github.com/tmc/md2html/internal/markdown/components"
	"github.com/tmc/md2html/internal/markdown/jsonspec"
)

type Config struct {
	// Chdir is the directory relative paths resolve against, like
	// go -C or make -C: the source, output, css, and configuration
	// files. Unlike those commands, Run resolves against it rather
	// than changing the process working directory, so an embedding
	// caller keeps its own. It is validated even when every named
	// path is absolute.
	Chdir             string
	Source            string // file, directory, or "-" for stdin
	HTTP              string
	HTML              string
	Open              bool
	Verbose           bool
	Title             string
	CSS               string
	Depth             int // directory traversal depth; values below 2 mean 2
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
}

// preparedSite holds loaded components, icons, JSON schemas, and their
// runtime state for a site build or server session.
type preparedSite struct {
	config Config
	// base is the absolute directory config's relative filesystem paths
	// were resolved against. It is kept because paths discovered later —
	// the source root, the icon search path, the repository git metadata
	// is read from — are anchored to it rather than to the process
	// working directory, which Run does not change.
	base string

	componentRegistry components.Registry
	iconSet           map[string]template.HTML
	iconStyles        map[string]map[string]template.HTML
	iconAttribution   string
	iconMissing       *sync.Map
	iconLogger        *slog.Logger
	iconDisabled      bool

	jsonSpecConfig jsonspec.Config
	jsonSpecBundle template.JS
	jsonSpecReady  bool

	starsAPI string
}

// newPreparedSite returns a site whose configuration paths are resolved
// against the base Config.Chdir names, with no resources loaded yet.
func newPreparedSite(cfg Config, logger *slog.Logger) (*preparedSite, error) {
	if logger == nil {
		logger = slog.Default()
	}
	base, err := resolveBase(cfg.Chdir)
	if err != nil {
		return nil, err
	}
	return &preparedSite{
		config:      resolveConfigPaths(cfg, base),
		base:        base,
		iconMissing: new(sync.Map),
		iconLogger:  logger,
	}, nil
}

func prepareSite(cfg Config, logger *slog.Logger) (*preparedSite, error) {
	if logger == nil {
		logger = slog.Default()
	}
	s, err := newPreparedSite(cfg, logger)
	if err != nil {
		return nil, err
	}
	if err := s.prepareJSONSpec(logger); err != nil {
		return nil, err
	}
	if err := s.prepareComponents(); err != nil {
		return nil, err
	}
	if err := s.prepareIcons(); err != nil {
		return nil, err
	}
	return s, nil
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
