package md2html

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

func newServer(ctx context.Context, cfg Config, logger *slog.Logger) *server {
	s := &server{
		ctx:        ctx,
		config:     cfg,
		logger:     logger,
		clients:    make(map[chan string]bool),
		inputPath:  cfg.Source,
		shutdownCh: make(chan struct{}),
	}

	// Initialize version management if enabled
	if cfg.Versions {
		wd, err := os.Getwd()
		if err != nil {
			logger.Error("Error getting working directory", "error", err)
		} else {
			s.versionMgr = newGitVersionManager(ctx, wd)
			if s.versionMgr.IsGitRepo() {
				versions, err := s.versionMgr.ListVersions(cfg.VersionBranches, cfg.VersionPattern)
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
		root, err := sourceRoot(cfg.Source)
		if err == nil {
			htmlExt := ""
			if cfg.HTMLExt != "" {
				htmlExt = "." + cfg.HTMLExt
			}
			nav, err := LoadNavigationOrAutoFromDir(root, htmlExt)
			if err != nil {
				logger.Error("Error loading navigation", "error", err)
			} else if nav != nil && len(nav.Items) > 0 {
				s.nav = nav
				logger.Info("Loaded navigation", "pages", len(nav.Flat))
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

type server struct {
	ctx        context.Context
	config     Config
	logger     *slog.Logger
	mu         sync.RWMutex
	content    string
	clients    map[chan string]bool
	clientsMu  sync.RWMutex
	cssContent string
	inputPath  string
	jsonData   any // Generic JSON data for templates
	shutdownCh chan struct{}
	watcher    *fsnotify.Watcher
	watchMu    sync.Mutex
	watched    map[string]bool

	// Batched reload management
	reloadPending bool
	reloadTimer   *time.Timer
	reloadTimerMu sync.Mutex

	// Version management
	versionMgr *GitVersionManager
	versions   []GitVersion

	// Navigation from SUMMARY.md
	nav                *Navigation
	lastUpdatedMu      sync.Mutex
	lastUpdated        map[string]string
	gitMetadataRoot    string
	gitMetadataChecked bool
	gitMetadataOK      bool
}

func (s *server) startWatching() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
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
		root, err := sourceRoot(s.inputPath)
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
	if s.config.Verbose {
		s.watchMu.Lock()
		n := len(s.watched)
		s.watchMu.Unlock()
		s.logger.Debug("Watch setup complete", "paths", n)
	}

	go s.watchEvents(watcher)
	return nil
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

func (s *server) watchEvents(watcher *fsnotify.Watcher) {
	defer watcher.Close()
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			s.handleWatchEvent(event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		case <-s.shutdownCh:
			return
		}
	}
}

func (s *server) handleWatchEvent(event fsnotify.Event) {
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
	if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown") {
		s.notifyClients()
	}
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
	s.mu.RLock()
	content := s.content
	css := s.cssContent
	s.mu.RUnlock()

	root, err := sourceRoot(s.inputPath)
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

		if requestedVersion != "" && s.versionMgr != nil {
			fileContent, err = s.versionMgr.GetFileContent(requestedVersion, s.config.Index)
		} else {
			fileContent, err = os.ReadFile(s.config.Index)
		}

		if err == nil {
			if requestedVersion == "" {
				_ = s.watchOpenedPath(s.config.Index)
			}
			doc, err := parseFrontmatter(string(fileContent))
			if err != nil {
				log.Printf("Error parsing frontmatter in %s: %v", s.config.Index, err)
				doc = DocumentData{Content: string(fileContent), Frontmatter: make(map[string]any)}
			}
			html := s.renderDocumentWithVersion(doc, s.config.Index, css, s.config.Index, requestedVersion)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(html))
			return
		} else if s.config.Verbose {
			log.Printf("Index file %s not found, falling back to directory listing", s.config.Index)
		}
	}

	// Check if a specific file is requested via clean URL path
	if filePath != "" && filePath != "/" {
		cleanPath, err := secureURLPath(filePath)
		if err != nil {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
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
				fileContent, err = s.versionMgr.GetFileContent(requestedVersion, candidate)
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
					log.Printf("Error parsing frontmatter in %s: %v", candidate, err)
					doc = DocumentData{Content: string(fileContent), Frontmatter: make(map[string]any)}
				}
				html := s.renderDocumentWithVersion(doc, candidate, css, candidate, requestedVersion)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write([]byte(html))
				return
			}
		}

		// Fall back to serving the path as a static asset relative to the source root.
		fullPath, joinErr := secureJoin(root, filepath.FromSlash(cleanPath))
		if joinErr == nil {
			if info, statErr := os.Stat(fullPath); statErr == nil && !info.IsDir() {
				_ = s.watchOpenedPath(fullPath)
				http.ServeFile(w, r, fullPath)
				return
			}
		}

		// File not found
		http.Error(w, fmt.Sprintf("File not found: %s", filePath), http.StatusNotFound)
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
			http.Error(w, fmt.Sprintf("Error reading file %s: %v", file, err), http.StatusNotFound)
			return
		}
		_ = s.watchOpenedPath(fullPath)

		doc, err := parseFrontmatter(string(content))
		if err != nil {
			log.Printf("Error parsing frontmatter in %s: %v", file, err)
			doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
		}
		html := s.renderDocumentWithVersion(doc, file, css, file, requestedVersion)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
		return
	}

	// If no input file specified and content is empty, show directory listing
	if s.inputPath == "" && content == "" {
		wd, err := os.Getwd()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting working directory: %v", err), http.StatusInternalServerError)
			return
		}

		listing, err := generateDirectoryListing(s.config, wd)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error generating directory listing: %v", err), http.StatusInternalServerError)
			return
		}

		doc := DocumentData{Content: listing, Frontmatter: make(map[string]any)}
		html := s.renderDocumentWithVersion(doc, "Directory Listing", css, "", "")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
		return
	}

	doc, err := parseFrontmatter(content)
	if err != nil {
		log.Printf("Error parsing frontmatter: %v", err)
		doc = DocumentData{Content: content, Frontmatter: make(map[string]any)}
	}
	html := s.renderDocumentWithVersion(doc, s.config.Title, css, "", "")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// renderDocumentWithVersion renders a document with version information
func (s *server) renderDocumentWithVersion(doc DocumentData, title, customCSS, filePath, version string) string {
	html := markdownToHTMLWithContext(s.config, doc.Content, filePath)
	if s.config.RenderFrontmatter {
		html = renderFrontmatterHTML(doc.Frontmatter) + html
	}

	opts := RenderOptions{
		SiteTitle:   s.config.Title,
		Data:        s.jsonData,
		FilePath:    filePath,
		Version:     version,
		Versions:    s.versions,
		Description: llmsSummary(doc),
		EditURL:     editURL(s.config.EditURL, filePath),
	}
	opts.LastUpdated = s.lastUpdatedFor(filePath)
	if s.nav != nil {
		opts.Nav = s.nav.ForPage(filePath)
	}

	return renderTemplateWithOptions(s.config, html, title, customCSS, s.watchEnabled(), doc.Frontmatter, opts)
}

func (s *server) watchEnabled() bool {
	watch, err := watchEnabled(s.config.Watch, s.config.Source)
	return err == nil && watch
}

func (s *server) lastUpdatedFor(filePath string) string {
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

	root, err := sourceRoot(s.inputPath)
	if err != nil {
		return ""
	}
	if !s.gitMetadataAvailable(root) {
		return ""
	}
	times, err := gitLastUpdatedPaths(s.ctx, root, []string{filePath})
	if err != nil {
		if s.config.Verbose {
			s.logger.Debug("git metadata unavailable", "error", err)
		}
		return ""
	}
	stamp := times[filePath]

	s.lastUpdatedMu.Lock()
	s.lastUpdated[filePath] = stamp
	s.lastUpdatedMu.Unlock()
	return stamp
}

func (s *server) gitMetadataAvailable(root string) bool {
	s.lastUpdatedMu.Lock()
	if s.gitMetadataChecked && s.gitMetadataRoot == root {
		ok := s.gitMetadataOK
		s.lastUpdatedMu.Unlock()
		return ok
	}
	s.lastUpdatedMu.Unlock()

	ok := gitHasHead(s.ctx, root)

	s.lastUpdatedMu.Lock()
	s.gitMetadataRoot = root
	s.gitMetadataChecked = true
	s.gitMetadataOK = ok
	s.lastUpdatedMu.Unlock()
	return ok
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
	root, err := sourceRoot(s.inputPath)
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

func (s *server) handleRaw(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	content := s.content
	s.mu.RUnlock()

	html := markdownToHTMLWithContext(s.config, content, "")
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
		log.Printf("Notifying %d clients", len(s.clients))
	}

	for ch := range s.clients {
		select {
		case ch <- "reload":
		default:
		}
	}
}

// Run starts the server and handles graceful shutdown
func (s *server) Run(ctx context.Context) error {
	// Set up file watching
	if watch, err := watchEnabled(s.config.Watch, s.config.Source); err != nil {
		return err
	} else if watch {
		if err := s.startWatching(); err != nil {
			return err
		}
	}

	// Setup HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/raw", s.handleRaw)
	mux.HandleFunc("/api/versions", s.handleVersionsAPI)
	mux.HandleFunc("/_jsonspec/schemas.json", s.handleJSONSpecSchemas)

	// Register search routes ahead of handleIndex so the embedded assets win
	// over any js/ directory or stale search-index.js in the source tree.
	if s.config.Search {
		mux.HandleFunc("/js/minisearch.min.js", handleSearchAsset("static/js/minisearch.min.js"))
		mux.HandleFunc("/js/search.js", handleSearchAsset("static/js/search.js"))
		mux.HandleFunc("/search-index.js", s.handleSearchIndex)
	}

	srv := &http.Server{
		Addr:    s.config.HTTP,
		Handler: mux,
	}

	// Format URL for display and browser opening
	displayURL := formatServerURL(s.config.HTTP)

	// Open browser if requested
	if s.config.Open {
		go func() {
			if !openBrowser(ctx, displayURL) {
				s.logger.Warn("Failed to open browser", "url", displayURL)
			} else {
				s.logger.Debug("Opened browser", "url", displayURL)
			}
		}()
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		<-ctx.Done()
		// Close shutdown channel to notify all goroutines
		close(s.shutdownCh)
		// Send shutdown signal to all clients
		s.clientsMu.Lock()
		for client := range s.clients {
			select {
			case client <- "shutdown":
			default:
			}
		}
		s.clients = make(map[chan string]bool)
		s.clientsMu.Unlock()

		// Shutdown server with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("Server shutdown error", "error", err)
		}
		close(idleConnsClosed)
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		s.logger.Error("Server error", "error", err)
		return fmt.Errorf("server error: %v", err)
	}

	<-idleConnsClosed
	return nil
}
