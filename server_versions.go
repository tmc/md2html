package md2html

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tmc/md2html/internal/docversions"
	"github.com/tmc/md2html/internal/markdownfile"
)

// versionMounts serves the versions of a versioned site other than the
// working tree, each under /v/<name>/ from a worktree of its commit, so
// that its pages, navigation, includes, and search all come from the
// release. A version's worktree is made when it is first requested.
type versionMounts struct {
	site     *preparedSite // the working tree's site
	vs       *versionedSite
	manifest []byte
	page     []byte
	logger   *slog.Logger

	// moves maps the URL path, relative to the root, of each page that
	// moved since the newest release to the page's path now.
	moves map[string]string

	mu     sync.Mutex
	mounts map[string]*versionMount
	closed bool
}

// A versionMount is one version being served.
type versionMount struct {
	once    sync.Once
	handler http.Handler
	server  *server
	dir     string // temporary directory holding the worktree
	err     error
}

// versionPrefix is the URL path versions other than the working tree
// are served under.
const versionPrefix = "/v/"

// newVersionMounts plans the site's versions for serving. The working
// tree is served at the root as latest.
func newVersionMounts(ctx context.Context, site *preparedSite, logger *slog.Logger) (*versionMounts, error) {
	vs, err := site.newVersionedSite(ctx, func(name string) string {
		if name == latestVersion {
			return "./"
		}
		return strings.TrimPrefix(versionPrefix, "/") + name + "/"
	})
	if err != nil {
		return nil, err
	}
	// The server previews the working tree, at the root, so it is the
	// version the root opens whatever the static build's is.
	m := vs.manifest
	if d := site.config.VersionDefault; d != "" && d != latestVersion {
		logger.Warn("-version-default applies to static builds; serving the working tree at /", "version", d)
	}
	// The server shows an archive only from a directory.
	kept := m.Versions[:0]
	for _, v := range m.Versions {
		if v.Archive && strings.Contains(vs.archivePath(v.Name), "://") {
			logger.Warn("not serving remote archive", "version", v.Name)
			continue
		}
		if !v.Archive && site.config.Search {
			v.Search = "search-index.js"
		}
		kept = append(kept, v)
	}
	m.Versions = kept
	data, err := m.Marshal()
	if err != nil {
		return nil, err
	}
	site.version = vs.pageVersion(latestVersion, docversions.ManifestName)
	return &versionMounts{
		site:     site,
		vs:       vs,
		manifest: data,
		page:     versionsPage(m, site.config.Title),
		logger:   logger,
		moves:    vs.latestMoves(),
		mounts:   make(map[string]*versionMount),
	}, nil
}

// moved reports where the page at the URL path p, relative to the
// root, has moved to, as a URL path relative to the root. A nil
// versionMounts reports no moves.
func (vm *versionMounts) moved(p string) (string, bool) {
	if vm == nil {
		return "", false
	}
	for from, to := range vm.moves {
		if p == from || p == strings.TrimSuffix(from, path.Ext(from)) {
			if markdownfile.Is(p) {
				return to, true
			}
			return strings.TrimSuffix(to, path.Ext(to)), true
		}
	}
	return "", false
}

// archivePath returns the configured location of the archive name.
func (vs *versionedSite) archivePath(name string) string {
	for _, a := range vs.plan.config.Archive {
		if a.Name == name {
			if strings.Contains(a.Path, "://") || filepath.IsAbs(a.Path) {
				return a.Path
			}
			return filepath.Join(vs.plan.configDir, a.Path)
		}
	}
	return ""
}

// register adds the manifest, the versions page, and the versions to
// mux, which serves the working tree.
func (vm *versionMounts) register(mux *http.ServeMux) {
	mux.HandleFunc("/"+docversions.ManifestName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(vm.manifest)
	})
	mux.HandleFunc("/versions/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(vm.page)
	})
	mux.Handle(versionPrefix, vm)
}

func (vm *versionMounts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, versionPrefix)
	name, _, hasSlash := strings.Cut(rest, "/")
	v, ok := vm.vs.manifest.Lookup(name)
	if !ok || v.Name == latestVersion {
		http.NotFound(w, r)
		return
	}
	if !hasSlash {
		// Pages link relative to the version root, so it needs its
		// trailing slash.
		http.Redirect(w, r, name+"/", http.StatusMovedPermanently)
		return
	}
	prefix := versionPrefix + name
	if v.Archive {
		http.StripPrefix(prefix, http.FileServer(http.Dir(vm.vs.archivePath(name)))).ServeHTTP(w, r)
		return
	}
	mount, err := vm.mount(r.Context(), name)
	if err != nil {
		vm.logger.Error("serving version", "version", name, "error", err)
		http.Error(w, "version "+name+" is unavailable", http.StatusInternalServerError)
		return
	}
	http.StripPrefix(prefix, mount.handler).ServeHTTP(w, r)
}

// mount returns the version name, making its worktree and server the
// first time it is asked for.
func (vm *versionMounts) mount(ctx context.Context, name string) (*versionMount, error) {
	vm.mu.Lock()
	if vm.closed {
		vm.mu.Unlock()
		return nil, errors.New("server is stopping")
	}
	m := vm.mounts[name]
	if m == nil {
		m = new(versionMount)
		vm.mounts[name] = m
	}
	vm.mu.Unlock()

	// The worktree outlives the request that happened to ask for it
	// first.
	m.once.Do(func() { m.err = vm.start(context.WithoutCancel(ctx), name, m) })
	return m, m.err
}

func (vm *versionMounts) start(ctx context.Context, name string, m *versionMount) error {
	ref, _ := vm.vs.plan.ref(name)
	cfg := vm.site.config
	parent := ""
	if cfg.VersionCache != "" {
		parent = filepath.Join(cfg.VersionCache, "worktrees")
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	dir, err := os.MkdirTemp(parent, "md2html-"+name+"-")
	if err != nil {
		return err
	}
	m.dir = dir
	wt := filepath.Join(dir, "src")
	vm.logger.Info("checking out version", "version", name, "commit", shortCommit(ref.Commit))
	if err := vm.vs.plan.repo.AddWorktree(ctx, wt, ref.Commit); err != nil {
		return err
	}
	cfg.Watch = "false"
	cfg.Open = false
	vsite, err := vm.site.refSite(cfg, vm.vs.plan, wt, vm.vs.pageVersion(name, "../../"+docversions.ManifestName), vm.logger)
	if err != nil {
		return err
	}
	m.server = newServer(ctx, vsite, vm.logger)
	// The server finds the -index page relative to its base, which
	// for a version is the same directory in the version's worktree.
	m.server.base = vm.vs.plan.remap(vm.site.base, wt)
	m.handler = m.server.contentMux()
	return nil
}

// close stops the versions' servers and removes their worktrees.
func (vm *versionMounts) close() {
	vm.mu.Lock()
	vm.closed = true
	mounts := vm.mounts
	vm.mu.Unlock()
	for _, m := range mounts {
		m.once.Do(func() { m.err = errors.New("server is stopping") })
		if m.server != nil {
			m.server.stop()
		}
		if m.dir != "" {
			vm.vs.plan.repo.RemoveWorktree(context.Background(), filepath.Join(m.dir, "src"))
			os.RemoveAll(m.dir)
		}
	}
}
