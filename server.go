package md2html

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

func newServer(ctx context.Context, site *preparedSite, logger *slog.Logger) *server {
	if site == nil {
		site = &preparedSite{}
	}
	cfg := site.config
	s := &server{
		config:     cfg,
		base:       site.base,
		prepared:   site,
		logger:     logger,
		clients:    make(map[chan string]bool),
		inputPath:  cfg.Source,
		shutdownCh: make(chan struct{}),
	}

	// Initialize version management if enabled
	if cfg.Versions {
		// Versions come from the repository the base directory is in,
		// not from the source tree, which may be a subdirectory of it or
		// not versioned at all.
		s.versionMgr = NewGitVersionManager(s.base)
		if s.versionMgr.isGitRepo(ctx) {
			versions, err := s.versionMgr.listVersions(ctx, cfg.VersionBranches, cfg.VersionPattern)
			if err != nil {
				logger.Error("Error listing versions", "error", err)
			} else {
				s.versions = versions
				logger.Info("Loaded versions", "count", len(versions))
			}
		} else {
			logger.Warn("Versions enabled but not in a git repository")
		}
	}

	// Load JSON data if provided
	if cfg.DataJSON != "" {
		jsonData, err := loadJSONFile(cfg.DataJSON)
		if err != nil {
			logger.Error("Error loading JSON data file", "error", err, "file", cfg.DataJSON)
		} else {
			s.jsonData = jsonData
			logger.Debug("Loaded JSON data", "file", cfg.DataJSON)
		}
	}

	// Load navigation from SUMMARY.md or build it from the markdown tree.
	if cfg.Nav {
		root, err := sourceRoot(s.base, cfg.Source)
		if err == nil {
			htmlExt := ""
			if cfg.HTMLExt != "" {
				htmlExt = "." + cfg.HTMLExt
			}
			s.navRoot = root
			s.navTitle = s.config.Title
			if siteDir, found := findDocsJSON(root); found {
				s.navConfig = filepath.Join(siteDir, docsJSONName)
			}
			nav, site, err := navigationForDir(root, htmlExt)
			if err != nil {
				logger.Error("Error loading navigation", "error", err)
			} else if nav != nil && len(nav.Items) > 0 {
				s.nav = nav
				s.site = site
				s.title = siteTitle(s.config.Title, site.Name)
				logger.Info("Loaded navigation", "pages", len(nav.Flat))
				s.site.Stars = s.prepared.repoStars(ctx, site.Repo, logger)
			}
		}
	}

	// Load initial content
	if cfg.Source != "" && cfg.Source != "-" {
		content, err := os.ReadFile(cfg.Source)
		if err != nil {
			logger.Error("Error reading initial file", "error", err, "file", cfg.Source)
		} else {
			s.mu.Lock()
			s.content = string(content)
			s.mu.Unlock()
			logger.Debug("Loaded initial content", "file", cfg.Source)
		}
	}

	// Load CSS if provided
	if cfg.CSS != "" {
		css, err := os.ReadFile(cfg.CSS)
		if err != nil {
			logger.Error("Error reading CSS file", "error", err, "file", cfg.CSS)
		} else {
			s.cssContent = string(css)
			logger.Debug("Loaded CSS content", "file", cfg.CSS)
		}
	}

	return s
}

// A server serves one source tree over HTTP.
//
// It keeps no context of its own. Work that belongs to a request uses
// the request's context, and work that lasts as long as the server uses
// the one passed to [server.Run]; startup work in newServer uses its
// caller's and does not outlive it.
type server struct {
	config Config
	// base is the directory the configuration's relative paths were
	// resolved against. Requests resolve against it too, so serving does
	// not depend on the process working directory.
	base       string
	prepared   *preparedSite
	logger     *slog.Logger
	mu         sync.RWMutex
	content    string
	clients    map[chan string]bool
	clientsMu  sync.RWMutex
	cssContent string
	inputPath  string
	jsonData   any // Generic JSON data for templates
	// shutdownCh is closed once, by Run, to release background work and
	// tell connected browsers the server is going away.
	shutdownCh   chan struct{}
	shutdownOnce sync.Once
	// stopRequests cancels the contexts of requests in flight. Run sets
	// it; stop calls it, after the live-reload clients have been told,
	// so a stream always sees the shutdown message before its request
	// context goes away.
	stopRequests context.CancelFunc
	watcher      *fsnotify.Watcher
	watchMu      sync.Mutex
	watched      map[string]bool

	// Version management
	versionMgr *GitVersionManager
	versions   []GitVersion

	// Navigation and the site presentation its source carries. Both are
	// rebuilt when their source changes, so they are read under mu.
	nav       *Navigation
	site      siteInfo
	navRoot   string // source tree navigation is built from
	navConfig string // docs.json covering that tree, when there is one
	// navTitle is the title the caller configured, kept so a reload can
	// recompute the effective title. Without it the site name applied at
	// startup would look like a caller's choice and outrank every later
	// one, so a renamed site never took effect.
	navTitle string
	// title is the effective site title. It is derived from navTitle and
	// the site name, so a reload can change it while requests are being
	// served; read it with siteTitle rather than config.Title.
	title         string
	lastUpdatedMu sync.Mutex
	lastUpdated   map[string]string
	gitMetadata   map[string]gitProbe
}

// sourceRoot returns the directory requests are served from.
func (s *server) sourceRoot() (string, error) {
	return sourceRoot(s.base, s.inputPath)
}

// startWatching registers the paths whose changes reload the page and
// returns the watcher. Run owns it from there: it decides when the
// events are consumed and when the watcher is closed.
func (s *server) startWatching() (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	s.watchMu.Lock()
	s.watcher = watcher
	s.watched = make(map[string]bool)
	s.watchMu.Unlock()

	if s.inputPath != "" && s.inputPath != "-" {
		if err := s.watchOpenedPath(s.inputPath); err != nil && s.config.Verbose {
			s.logger.Debug("watch source", "path", s.inputPath, "error", err)
		}
	} else {
		root, err := s.sourceRoot()
		if err == nil {
			if err := s.watchPath(root); err != nil && s.config.Verbose {
				s.logger.Debug("watch root", "path", root, "error", err)
			}
		}
	}
	if s.config.CSS != "" {
		if err := s.watchOpenedPath(s.config.CSS); err != nil && s.config.Verbose {
			s.logger.Debug("watch css", "path", s.config.CSS, "error", err)
		}
	}
	// docs.json sits at the root of the published site, which is often
	// above the directory being served, so it needs a watch of its own.
	if s.navConfig != "" {
		if err := s.watchOpenedPath(s.navConfig); err != nil && s.config.Verbose {
			s.logger.Debug("watch navigation config", "path", s.navConfig, "error", err)
		}
	}
	if s.config.Verbose {
		s.watchMu.Lock()
		n := len(s.watched)
		s.watchMu.Unlock()
		s.logger.Debug("Watch setup complete", "paths", n)
	}

	return watcher, nil
}

func (s *server) watchOpenedPath(path string) error {
	if path == "" {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := s.watchPath(filepath.Dir(abs)); err != nil {
		return err
	}
	return s.watchPath(abs)
}

func (s *server) watchPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watcher == nil {
		return nil
	}
	if s.watched[abs] {
		return nil
	}
	if err := s.watcher.Add(abs); err != nil {
		return err
	}
	s.watched[abs] = true
	if s.config.Verbose {
		s.logger.Debug("Watching path", "path", abs)
	}
	return nil
}

// watchEvents consumes watcher until the server's lifetime ends. It
// owns the watcher for that time and closes it on the way out.
func (s *server) watchEvents(ctx context.Context, watcher *fsnotify.Watcher) {
	defer watcher.Close()
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			s.handleWatchEvent(ctx, event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			s.logger.Error("Watcher error", "error", err)
		case <-ctx.Done():
			return
		case <-s.shutdownCh:
			return
		}
	}
}

func (s *server) handleWatchEvent(ctx context.Context, event fsnotify.Event) {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) == 0 {
		return
	}
	if s.config.Verbose {
		s.logger.Debug("Watch event", "op", event.Op.String(), "path", event.Name)
	}

	if samePath(event.Name, s.inputPath) {
		content, err := os.ReadFile(s.inputPath)
		if err == nil {
			s.mu.Lock()
			s.content = string(content)
			s.mu.Unlock()
		}
		s.notifyClients()
		return
	}
	if samePath(event.Name, s.config.CSS) {
		css, err := os.ReadFile(s.config.CSS)
		if err == nil {
			s.mu.Lock()
			s.cssContent = string(css)
			s.mu.Unlock()
		}
		s.notifyClients()
		return
	}

	name := strings.ToLower(event.Name)
	// A navigation source describes the whole site, so a change to it
	// changes every page, not only the one being viewed.
	if samePath(event.Name, s.navConfig) || strings.HasSuffix(name, "summary.md") {
		s.reloadNavigation(ctx)
		s.notifyClients()
		return
	}
	if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown") {
		s.notifyClients()
	}
}

// reloadNavigation rebuilds the navigation and the presentation its
// source carries, so an edit to docs.json is picked up without a
// restart.
//
// A rebuild that finds nothing leaves the previous navigation in place:
// a config saved halfway through an edit should not empty the sidebar.
// The star count is carried over rather than refetched, since editing
// the file that names the repository is not news about how many stars it
// has, and a request per keystroke would be rate limited in short order.
func (s *server) reloadNavigation(ctx context.Context) {
	if s.navRoot == "" {
		return
	}
	htmlExt := ""
	if s.config.HTMLExt != "" {
		htmlExt = "." + s.config.HTMLExt
	}
	nav, site, err := navigationForDir(s.navRoot, htmlExt)
	if err != nil {
		s.logger.Warn("Could not reload navigation", "error", err)
		return
	}
	if nav == nil || len(nav.Items) == 0 {
		s.logger.Warn("Reloaded navigation is empty; keeping the previous one")
		return
	}

	// The star count is fetched before the lock is taken. It is a network
	// call bounded only by its own timeout, and every request rendering a
	// page reads this state, so fetching it under the write lock would
	// stall the whole server for as long as GitHub takes to answer.
	s.mu.RLock()
	previous := s.site
	s.mu.RUnlock()
	if site.Repo == previous.Repo {
		site.Stars = previous.Stars
	} else {
		site.Stars = s.prepared.repoStars(ctx, site.Repo, s.logger)
	}

	s.mu.Lock()
	s.nav = nav
	s.site = site
	s.title = siteTitle(s.navTitle, site.Name)
	s.mu.Unlock()

	s.logger.Info("Reloaded navigation", "pages", len(nav.Flat))
}

// siteTitle returns the effective site title. Navigation reload can
// change it while requests are in flight, so it is read under the lock
// rather than from config.
func (s *server) siteTitle() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.title == "" {
		return s.config.Title
	}
	return s.title
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return aa == bb
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.mu.RLock()
	content := s.content
	css := s.cssContent
	s.mu.RUnlock()

	root, err := s.sourceRoot()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error resolving source root: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract version from URL if versioning is enabled
	var requestedVersion string
	var filePath string
	if s.config.Versions && len(s.versions) > 0 {
		// URL format: /v/{version}/{path} or /{path}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if len(parts) > 1 && parts[0] == "v" {
			requestedVersion = parts[1]
			filePath = strings.Join(parts[2:], "/")
		} else {
			filePath = strings.Join(parts, "/")
			// Use default version or current
			if s.config.VersionDefault != "" {
				requestedVersion = s.config.VersionDefault
			} else if len(s.versions) > 0 {
				requestedVersion = s.versions[0].Name
			}
		}
	} else {
		filePath = strings.TrimPrefix(r.URL.Path, "/")
	}

	// Check if root path and index file is specified
	if (filePath == "" || filePath == "/") && s.config.Index != "" {
		var fileContent []byte
		var err error

		indexPath := resolveAgainst(s.base, s.config.Index)
		if requestedVersion != "" && s.versionMgr != nil {
			fileContent, err = s.versionMgr.fileContent(ctx, requestedVersion, s.config.Index)
		} else {
			fileContent, err = os.ReadFile(indexPath)
		}

		if err == nil {
			if requestedVersion == "" {
				_ = s.watchOpenedPath(indexPath)
			}
			doc, err := parseFrontmatter(string(fileContent))
			if err != nil {
				s.logger.Error("Error parsing frontmatter", "file", s.config.Index, "error", err)
				doc = DocumentData{Content: string(fileContent), Frontmatter: make(map[string]any)}
			}
			html, err := s.renderDocumentWithVersion(ctx, doc, documentTitle(doc, s.config.Index, s.siteTitle()), css, s.config.Index, requestedVersion)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(html))
			return
		} else if s.config.Verbose {
			s.logger.Warn("Index file not found, falling back to directory listing", "file", s.config.Index)
		}
	}

	// Check if a specific file is requested via clean URL path
	if filePath != "" && filePath != "/" {
		cleanPath, err := secureURLPath(filePath)
		if err != nil {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		// A page the repository excludes from its site is not served
		// here either. Listing it nowhere but answering for it anyway
		// would still publish it to anyone holding the URL.
		if ignore, err := loadIgnoreSet(root); err == nil && ignore != nil {
			if full, joinErr := secureJoin(root, filepath.FromSlash(cleanPath)); joinErr == nil {
				info, statErr := os.Stat(full)
				if ignore.excludes(full, statErr == nil && info.IsDir()) {
					s.serveNotFound(ctx, w, requestedURL(r), css)
					return
				}
			}
		}

		// Try the path as-is if it ends with .md
		var candidates []string
		if strings.HasSuffix(cleanPath, ".md") || strings.HasSuffix(cleanPath, ".markdown") {
			candidates = append(candidates, cleanPath)
		} else {
			// Try adding .md extension
			candidates = append(candidates, cleanPath+".md")
			candidates = append(candidates, cleanPath+".markdown")
		}

		for _, candidate := range candidates {
			var fileContent []byte
			var fullPath string
			var err error

			if requestedVersion != "" && s.versionMgr != nil {
				fileContent, err = s.versionMgr.fileContent(ctx, requestedVersion, candidate)
			} else {
				var joinErr error
				fullPath, joinErr = secureJoin(root, filepath.FromSlash(candidate))
				if joinErr != nil {
					http.Error(w, "invalid path", http.StatusBadRequest)
					return
				}
				fileContent, err = os.ReadFile(fullPath)
			}

			if err == nil {
				if requestedVersion == "" {
					_ = s.watchOpenedPath(fullPath)
				}
				doc, err := parseFrontmatter(string(fileContent))
				if err != nil {
					s.logger.Error("Error parsing frontmatter", "file", candidate, "error", err)
					doc = DocumentData{Content: string(fileContent), Frontmatter: make(map[string]any)}
				}
				html, err := s.renderDocumentWithVersion(ctx, doc, documentTitle(doc, candidate, s.siteTitle()), css, candidate, requestedVersion)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write([]byte(html))
				return
			}
		}

		// Fall back to serving the path as a static asset relative to the
		// source root, or as a directory page when it names a directory.
		fullPath, joinErr := secureJoin(root, filepath.FromSlash(cleanPath))
		if joinErr == nil {
			if info, statErr := os.Stat(fullPath); statErr == nil {
				if info.IsDir() {
					// Listing links are relative to the directory, so the
					// URL needs its trailing slash for them to resolve.
					if !strings.HasSuffix(r.URL.Path, "/") {
						http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
						return
					}
					s.serveDirectory(ctx, w, fullPath, strings.TrimSuffix(cleanPath, "/"), css, requestedVersion)
					return
				}
				_ = s.watchOpenedPath(fullPath)
				http.ServeFile(w, r, fullPath)
				return
			}
		}

		s.serveNotFound(ctx, w, requestedURL(r), css)
		return
	}

	// Check if a specific file is requested via query parameter (for backward compatibility)
	if file := r.URL.Query().Get("file"); file != "" {
		cleanPath, err := secureURLPath(file)
		if err != nil {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		fullPath, err := secureJoin(root, filepath.FromSlash(cleanPath))
		if err != nil {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			s.serveNotFound(ctx, w, requestedURL(r), css)
			return
		}
		_ = s.watchOpenedPath(fullPath)

		doc, err := parseFrontmatter(string(content))
		if err != nil {
			s.logger.Error("Error parsing frontmatter", "file", file, "error", err)
			doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
		}
		html, err := s.renderDocumentWithVersion(ctx, doc, documentTitle(doc, file, s.siteTitle()), css, file, requestedVersion)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
		return
	}

	// If no input file specified and content is empty, serve the root
	// directory: its index file if it has one, otherwise a listing.
	if s.inputPath == "" && content == "" {
		s.serveDirectory(ctx, w, root, "", css, "")
		return
	}

	doc, err := parseFrontmatter(content)
	if err != nil {
		s.logger.Error("Error parsing frontmatter", "error", err)
		doc = DocumentData{Content: content, Frontmatter: make(map[string]any)}
	}
	html, err := s.renderDocumentWithVersion(ctx, doc, s.siteTitle(), css, "", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// dirIndexNames are the file names tried, in order, when a request resolves
// to a directory. The configured index file wins when one is set.
func (s *server) dirIndexNames() []string {
	names := []string{"index.md", "index.markdown", "README.md", "readme.md", "SKILL.md"}
	if s.config.Index != "" {
		names = append([]string{s.config.Index}, names...)
	}
	return names
}

// serveDirectory serves the directory at dir, whose path relative to the
// served root is relDir. A directory with an index file renders that file;
// one without renders a listing of the Markdown files beneath it.
func (s *server) serveDirectory(ctx context.Context, w http.ResponseWriter, dir, relDir, css, version string) {
	for _, name := range s.dirIndexNames() {
		indexPath := filepath.Join(dir, filepath.FromSlash(name))
		content, err := os.ReadFile(indexPath)
		if err != nil {
			continue
		}
		_ = s.watchOpenedPath(indexPath)
		relIndex := path.Join(filepath.ToSlash(relDir), name)
		doc, err := parseFrontmatter(string(content))
		if err != nil {
			s.logger.Error("Error parsing frontmatter", "file", relIndex, "error", err)
			doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
		}
		html, err := s.renderDocumentWithVersion(ctx, doc, documentTitle(doc, relIndex, s.siteTitle()), css, relIndex, version)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
		return
	}

	listing, err := generateDirectoryListing(s.config, dir, relDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating directory listing: %v", err), http.StatusInternalServerError)
		return
	}
	doc := DocumentData{Content: listing, Frontmatter: make(map[string]any)}
	html, err := s.renderDocumentWithVersion(ctx, doc, listingTitle(relDir), css, "", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// requestedURL returns the path the client asked for. Under -base the
// handler runs behind http.StripPrefix, so r.URL.Path has lost the
// prefix and would name a URL that does not exist; RequestURI is what
// arrived on the wire.
func requestedURL(r *http.Request) string {
	target := r.RequestURI
	if target == "" {
		return r.URL.Path
	}
	if i := strings.IndexByte(target, '?'); i >= 0 {
		target = target[:i]
	}
	return target
}

// serveNotFound reports that urlPath names nothing, rendered through the
// usual page template so the navigation and styling survive a wrong link.
// It names only what the client already sent: the paths md2html searched
// are server-side detail, and echoing them tells a visitor where the
// source tree lives on disk.
func (s *server) serveNotFound(ctx context.Context, w http.ResponseWriter, urlPath, css string) {
	body := fmt.Sprintf("# Not found\n\nNo page matches `%s`.\n", strings.ReplaceAll(urlPath, "`", ""))
	doc := DocumentData{Content: body, Frontmatter: make(map[string]any)}
	html, err := s.renderDocumentWithVersion(ctx, doc, "Not found", css, "", "")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(html))
}

// renderDocumentWithVersion renders a document with version information
func (s *server) renderDocumentWithVersion(ctx context.Context, doc DocumentData, title, customCSS, filePath, version string) (string, error) {
	html, err := s.prepared.markdownToHTML(promoteTitleHeading(doc), filePath)
	if err != nil {
		return "", err
	}
	if s.config.RenderFrontmatter {
		html = renderFrontmatterHTML(doc.Frontmatter) + html
	}

	s.mu.RLock()
	nav, site := s.nav, s.site
	s.mu.RUnlock()

	opts := RenderOptions{
		SiteTitle:   s.siteTitle(),
		Accent:      site.Accent,
		AccentDark:  site.AccentDark,
		Repo:        site.Repo,
		RepoURL:     site.RepoURL,
		NavLinks:    site.Links,
		Stars:       site.Stars,
		ShowStars:   s.config.Stars,
		Data:        s.jsonData,
		FilePath:    filePath,
		Version:     version,
		Versions:    s.versions,
		Description: llmsSummary(doc),
		EditURL:     editURL(s.config.EditURL, filePath),
	}
	opts.LastUpdated = s.lastUpdatedFor(ctx, filePath)
	if nav != nil {
		opts.Nav = nav.ForPage(filePath)
	}

	return s.prepared.renderTemplate(html, title, customCSS, s.watchEnabled(), doc.Frontmatter, opts)
}

func (s *server) watchEnabled() bool {
	watch, err := watchEnabled(s.config.Watch, s.config.Source)
	return err == nil && watch
}

func (s *server) lastUpdatedFor(ctx context.Context, filePath string) string {
	filePath = filepath.ToSlash(filePath)
	if filePath == "" {
		return ""
	}

	s.lastUpdatedMu.Lock()
	if s.lastUpdated != nil {
		if stamp, ok := s.lastUpdated[filePath]; ok {
			s.lastUpdatedMu.Unlock()
			return stamp
		}
	} else {
		s.lastUpdated = make(map[string]string)
	}
	s.lastUpdatedMu.Unlock()

	root, err := s.sourceRoot()
	if err != nil {
		return ""
	}
	if !s.gitMetadataAvailable(ctx, root) {
		return ""
	}
	times, err := gitLastUpdatedPaths(ctx, root, []string{filePath})
	if err != nil {
		if s.config.Verbose {
			s.logger.Debug("git metadata unavailable", "error", err)
		}
		return ""
	}
	// A request that went away tells us nothing about the file. Caching
	// the empty answer it produced would leave the page dateless for as
	// long as the server ran.
	if ctx.Err() != nil {
		return ""
	}
	stamp := times[filePath]

	s.lastUpdatedMu.Lock()
	s.lastUpdated[filePath] = stamp
	s.lastUpdatedMu.Unlock()
	return stamp
}

// A gitProbe records what asking a directory for git metadata found.
// Only a probe that finished is worth remembering.
type gitProbe int

const (
	gitUnknown gitProbe = iota
	gitAbsent
	gitPresent
)

// gitMetadataAvailable reports whether root has the history page dates
// are read from.
//
// The answer is cached, but only when the probe ran to completion: a
// request canceled while git was still running says nothing about the
// repository, and remembering its failure would leave every later page
// without a date. The subprocess runs outside the lock, so a duplicate
// concurrent probe is possible; that is cheaper than coordinating to
// prevent one.
func (s *server) gitMetadataAvailable(ctx context.Context, root string) bool {
	s.lastUpdatedMu.Lock()
	known := s.gitMetadata[root]
	s.lastUpdatedMu.Unlock()
	if known != gitUnknown {
		return known == gitPresent
	}

	result := gitPresent
	if !gitHasHead(ctx, root) {
		if ctx.Err() != nil {
			return false
		}
		result = gitAbsent
	}

	s.lastUpdatedMu.Lock()
	if s.gitMetadata == nil {
		s.gitMetadata = make(map[string]gitProbe)
	}
	s.gitMetadata[root] = result
	s.lastUpdatedMu.Unlock()
	return result == gitPresent
}

// handleSearchAsset serves an embedded JS asset (minisearch, search.js) as
// application/javascript. The asset name is the path inside the embed.FS, e.g.
// "static/js/search.js".
func handleSearchAsset(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := readSearchAsset(name)
		if err != nil {
			http.Error(w, "asset not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(body)
	}
}

// handleSearchIndex builds the search index on every request so live edits
// show up without a server restart. The index is small enough that the cost
// is negligible for a development server.
func (s *server) handleSearchIndex(w http.ResponseWriter, r *http.Request) {
	root, err := s.sourceRoot()
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve source root: %v", err), http.StatusInternalServerError)
		return
	}
	docs, err := buildSearchIndex(root, s.config)
	if err != nil {
		http.Error(w, fmt.Sprintf("build search index: %v", err), http.StatusInternalServerError)
		return
	}
	body, err := renderSearchIndexJS(docs)
	if err != nil {
		http.Error(w, fmt.Sprintf("render search index: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(body)
}

// llmsPages gathers the pages behind llms.txt and llms-full.txt. Like
// the search index it is rebuilt per request, so an edit shows up
// without restarting the server.
func (s *server) llmsPages() ([]llmsPage, error) {
	root, err := s.sourceRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve source root: %w", err)
	}
	files, err := findMarkdownFiles(root, s.config.Depth)
	if err != nil {
		return nil, fmt.Errorf("find markdown files: %w", err)
	}
	htmlExt := ""
	if s.config.HTMLExt != "" {
		htmlExt = "." + s.config.HTMLExt
	}
	nav, _, err := navigationForDir(root, htmlExt)
	if err != nil {
		// Without navigation the pages are still listed, just ungrouped.
		nav = nil
	}
	return collectLLMSPages(root, files, nav, s.config)
}

func (s *server) handleLLMS(w http.ResponseWriter, r *http.Request) {
	pages, err := s.llmsPages()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	io.WriteString(w, buildLLMSSummary(s.config, pages))
}

func (s *server) handleLLMSFull(w http.ResponseWriter, r *http.Request) {
	pages, err := s.llmsPages()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	// The server has no reason to split: nothing here is being uploaded
	// to a host with a file-size limit.
	for i, p := range pages {
		if i > 0 {
			io.WriteString(w, "\n\n")
		}
		fmt.Fprintf(w, "--- %s\n\n%s", p.URL, p.Content)
	}
}

func (s *server) handleRaw(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	content := s.content
	s.mu.RUnlock()

	html, err := markdownToHTMLWithContext(s.config, content, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (s *server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client channel
	clientChan := make(chan string, 10)

	s.clientsMu.Lock()
	s.clients[clientChan] = true
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, clientChan)
		s.clientsMu.Unlock()
	}()

	// Keep connection alive
	fmt.Fprintf(w, "data: connected\n\n")
	w.(http.Flusher).Flush()

	// Create a local copy of shutdown channel to avoid panic
	shutdownCh := s.shutdownCh

	// Listen for updates
	for {
		select {
		case msg, ok := <-clientChan:
			if !ok {
				// Channel closed, server shutting down
				fmt.Fprintf(w, "data: shutdown\n\n")
				w.(http.Flusher).Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			// The server cancels requests when it stops. Say so, rather
			// than leaving the browser to notice the stream went quiet.
			select {
			case <-shutdownCh:
				fmt.Fprintf(w, "data: shutdown\n\n")
				w.(http.Flusher).Flush()
			default:
			}
			return
		case <-shutdownCh:
			// Server is shutting down
			fmt.Fprintf(w, "data: shutdown\n\n")
			w.(http.Flusher).Flush()
			return
		}
	}
}

func (s *server) notifyClients() {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	if s.config.Verbose {
		s.logger.Debug("Notifying clients", "count", len(s.clients))
	}

	for ch := range s.clients {
		select {
		case ch <- "reload":
		default:
		}
	}
}

// Run starts the server and handles graceful shutdown
// registerEndpoints registers the routes the rendered pages call by
// absolute URL: live reload, raw Markdown, the versions API, and the
// embedded search and JSON schema assets. The search assets are
// registered ahead of handleIndex so they win over any js/ directory or
// stale search-index.js in the source tree.
func (s *server) registerEndpoints(mux *http.ServeMux) {
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/raw", s.handleRaw)
	mux.HandleFunc("/llms.txt", s.handleLLMS)
	mux.HandleFunc("/llms-full.txt", s.handleLLMSFull)
	mux.HandleFunc("/api/versions", s.handleVersionsAPI)
	mux.HandleFunc("/_jsonspec/schemas.json", s.handleJSONSpecSchemas)

	if s.config.Search {
		mux.HandleFunc("/js/minisearch.min.js", handleSearchAsset("static/js/minisearch.min.js"))
		mux.HandleFunc("/js/search.js", handleSearchAsset("static/js/search.js"))
		mux.HandleFunc("/search-index.js", s.handleSearchIndex)
	}
	if s.prepared != nil && s.prepared.jsonSpecBundle != "" {
		mux.HandleFunc("/js/jsonspec.js", handleSearchAsset("static/js/jsonspec.js"))
	}
}

// Run serves until ctx is canceled. It owns every background worker it
// starts (the file watcher, the browser opener, the shutdown) and
// stops them on the way out whatever ends the call: a bad option, a
// listener that will not bind, or ordinary cancellation. Cleanup is
// established before any of them start, so a failure part of the way
// through startup leaves nothing running.
func (s *server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer wg.Wait()
	defer s.stop()
	defer cancel()

	base, err := normalizeBasePath(s.config.Base)
	if err != nil {
		return err
	}

	// Set up file watching
	if watch, err := watchEnabled(s.config.Watch, s.config.Source); err != nil {
		return err
	} else if watch {
		watcher, err := s.startWatching()
		if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.watchEvents(ctx, watcher)
		}()
	}

	// Setup HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	s.registerEndpoints(mux)

	// Requests descend from a context the server stops, so work a
	// request started does not outlive the server it was made to. It is
	// not ctx itself: stop has to reach the live-reload streams before
	// their requests are canceled out from under them.
	reqCtx, stopRequests := context.WithCancel(context.WithoutCancel(ctx))
	s.stopRequests = stopRequests
	defer stopRequests()

	srv := &http.Server{
		Addr:        s.config.HTTP,
		Handler:     mountAt(base, mux, s.registerEndpoints),
		BaseContext: func(net.Listener) context.Context { return reqCtx },
	}

	// Format URL for display and browser opening
	displayURL := formatServerURL(s.config.HTTP) + base + "/"

	// Open browser if requested
	if s.config.Open {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !openBrowser(ctx, displayURL) {
				s.logger.Warn("Failed to open browser", "url", displayURL)
			} else {
				s.logger.Debug("Opened browser", "url", displayURL)
			}
		}()
	}

	idleConnsClosed := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(idleConnsClosed)
		<-ctx.Done()
		s.stop()

		// Shutdown drains connections, so it needs a context of its
		// own: the lifetime context is already canceled, and handing it
		// over would abandon them instead.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Server shutdown error", "error", err)
		}
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		s.logger.Error("Server error", "error", err)
		return fmt.Errorf("server error: %w", err)
	}

	<-idleConnsClosed
	return nil
}

// stop releases background work and tells connected browsers the server
// is going away. Run calls it, once, however the call ends.
func (s *server) stop() {
	s.shutdownOnce.Do(func() {
		close(s.shutdownCh)
		s.clientsMu.Lock()
		for client := range s.clients {
			select {
			case client <- "shutdown":
			default:
			}
		}
		s.clients = make(map[chan string]bool)
		s.clientsMu.Unlock()
		if s.stopRequests != nil {
			s.stopRequests()
		}
	})
}
