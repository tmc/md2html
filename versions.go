package md2html

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tmc/md2html/internal/docversions"
)

// latestVersion names the version built from the working tree.
const latestVersion = "latest"

// sharedDirName is the directory at the root of a versioned static
// site holding the content-named files its versions share.
const sharedDirName = "_shared"

// versionsPageDir is the directory at the root of a versioned site
// holding the page that lists its versions.
const versionsPageDir = "versions"

// reservedName reports whether name is used at the root of a versioned
// site for something other than a version. Case is ignored, as many
// file systems ignore it.
func reservedName(name string) bool {
	for _, r := range []string{latestVersion, versionsPageDir, sharedDirName, docversions.ManifestName} {
		if strings.EqualFold(name, r) {
			return true
		}
	}
	return false
}

// versionConfigName is the version policy file read from the source
// directory when -version-config names none.
const versionConfigName = "versions.yaml"

// pageVersion is what a page of a versioned site knows about the
// version it belongs to.
type pageVersion struct {
	name string
	// manifest is the URL of versions.json relative to the version
	// root.
	manifest string
	versions []GitVersion
	// edit maps the source path of each page of the version whose
	// path in the working tree differs to that path, or to "" if the
	// page is no longer there.
	edit map[string]string
}

// apply sets the version fields of opts. A nil pageVersion leaves them
// unset.
func (v *pageVersion) apply(opts *RenderOptions) {
	if v == nil {
		return
	}
	opts.Version = v.name
	opts.Versions = v.versions
	opts.VersionsURL = v.manifest
}

// editPath returns the source path the page relPath of the version is
// edited at, the page's path in the working tree, or "" if it is no
// longer there. Pages of a site that is not versioned, and of its
// working tree, are edited where they are.
func (v *pageVersion) editPath(relPath string) string {
	if v == nil {
		return relPath
	}
	if p, ok := v.edit[filepath.ToSlash(relPath)]; ok {
		return p
	}
	return relPath
}

// options returns opts with the version fields set.
func (v *pageVersion) options(opts RenderOptions) RenderOptions {
	v.apply(&opts)
	return opts
}

// A versionPlan is the set of versions a site publishes, read from its
// repository and version policy.
type versionPlan struct {
	repo   docversions.Repo
	top    string // repository root
	rel    string // source directory relative to top, slash-separated
	config *docversions.Config
	// configDir is the directory archive paths resolve against.
	configDir string
	refs      []docversions.Ref // published refs, newest first
	head      string            // commit at HEAD, or "" with none
}

// planVersions reads the versions the source directory publishes.
func (s *preparedSite) planVersions(ctx context.Context) (*versionPlan, error) {
	cfg := s.config
	sourceDir, err := filepath.Abs(s.sourceDir())
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("versions: source %s is not a directory", sourceDir)
	}
	repo := docversions.Repo{Dir: sourceDir}
	if !repo.IsRepo(ctx) {
		return nil, fmt.Errorf("versions: %s is not in a git repository", sourceDir)
	}
	top, err := repo.Toplevel(ctx)
	if err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}
	rel, err := docversions.RelPath(top, sourceDir)
	if err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}

	file := cfg.VersionConfig
	if file == "" {
		file = filepath.Join(sourceDir, versionConfigName)
	}
	vcfg, err := docversions.LoadConfig(file)
	if err != nil {
		return nil, err
	}
	plan := &versionPlan{
		repo:      repo,
		top:       top,
		rel:       rel,
		config:    vcfg,
		configDir: filepath.Dir(file),
	}
	plan.head, _ = repo.Commit(ctx, "HEAD")

	tags, err := repo.Tags(ctx, cfg.VersionPattern)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	// Branches are under development, so they come before the tags,
	// keeping the list newest first.
	var refs []docversions.Ref
	if cfg.VersionBranches {
		branches, err := repo.Branches(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing branches: %w", err)
		}
		// A release branch often has its release's name; the tag is
		// the version.
		tagged := make(map[string]bool, len(tags))
		for _, t := range tags {
			tagged[t.Name] = true
		}
		for _, b := range branches {
			if !tagged[b.Name] {
				refs = append(refs, b)
			}
		}
	}
	refs = append(refs, tags...)
	archived := make(map[string]bool)
	for _, a := range vcfg.Archive {
		if reservedName(a.Name) {
			return nil, fmt.Errorf("%s: archive name %q is reserved", file, a.Name)
		}
		archived[a.Name] = true
	}
	for _, r := range refs {
		switch {
		case reservedName(r.Name) || archived[r.Name] || docversions.CheckName(r.Name) != nil:
			continue
		case r.IsTag && !vcfg.Publishes(r.Name):
			continue
		}
		plan.refs = append(plan.refs, r)
	}
	return plan, nil
}

// manifest returns the manifest for the plan. versionPath gives each
// version's root relative to the manifest.
func (p *versionPlan) manifest(versionPath func(name string) string) *docversions.Manifest {
	m := &docversions.Manifest{
		Default: latestVersion,
		Banner:  p.config.BannerText(),
	}
	m.Versions = append(m.Versions, docversions.Version{
		Name:   latestVersion,
		Label:  p.config.Label(latestVersion),
		Path:   versionPath(latestVersion),
		Commit: p.head,
		State:  docversions.StateDev,
	})
	var tagNames []string
	for _, r := range p.refs {
		if r.IsTag {
			tagNames = append(tagNames, r.Name)
		}
	}
	hidden := p.config.Hidden(tagNames)
	// The newest release is stable. A prerelease newer than it is
	// still under development.
	stable := false
	for _, r := range p.refs {
		state := docversions.StateDev
		switch {
		case !r.IsTag:
		case !stable && docversions.Prerelease(r.Name):
		case !stable:
			state, stable = docversions.StateStable, true
		default:
			state = docversions.StateOld
		}
		m.Versions = append(m.Versions, docversions.Version{
			Name:   r.Name,
			Label:  p.config.Label(r.Name),
			Path:   versionPath(r.Name),
			Ref:    r.Ref,
			Commit: r.Commit,
			State:  state,
			Hidden: hidden[r.Name],
		})
	}
	for _, a := range p.config.Archive {
		label := a.Label
		if label == "" {
			label = p.config.Label(a.Name)
		}
		m.Versions = append(m.Versions, docversions.Version{
			Name:    a.Name,
			Label:   label,
			Path:    versionPath(a.Name),
			State:   docversions.StateOld,
			Archive: true,
		})
	}
	// Tags and branches sort among themselves; archives predate both.
	return m
}

// ref returns the published ref named name.
func (p *versionPlan) ref(name string) (docversions.Ref, bool) {
	for _, r := range p.refs {
		if r.Name == name {
			return r, true
		}
	}
	return docversions.Ref{}, false
}

// A pageFate is where a page of a version went by the working tree.
type pageFate struct {
	last string // the page's path when last seen: now, or where it was deleted
	now  string // its path in the working tree, or "" if it is gone
}

// moves returns, for each published ref, the source pages that moved
// between it and the working tree, mapping each page's path in the ref
// to its path now, or, for a page since deleted, to its path when it
// was. It also returns each ref's edit paths, as [pageVersion.edit]
// holds them.
//
// For tags, git compares each tag with the next newer one, and the
// newest with the working tree, and a page is followed through the
// steps from its tag: neighbouring releases differ little, so a page
// that moved and was then rewritten over several releases is still
// found. A deletion ends the page's path, so a later page at the same
// path is not taken for it. A branch is compared with the working tree
// directly. A page that is no page now and that a page of the working
// tree lists in its aliases is that page.
func (p *versionPlan) moves(ctx context.Context, aliases map[string]string, have map[string]bool) (moves, edits map[string]map[string]string) {
	moves = make(map[string]map[string]string)
	edits = make(map[string]map[string]string)
	// settle returns the fate of a page at path at the end of its
	// steps, deleted or not.
	settle := func(path string, deleted bool) pageFate {
		gone := deleted || !have[path]
		if to, ok := aliases[path]; ok && gone {
			return pageFate{last: path, now: to}
		}
		if gone {
			return pageFate{last: path}
		}
		return pageFate{last: path, now: path}
	}
	// resolve returns the fates of the pages of r, which reach the next
	// version by from and then fare as after says.
	resolve := func(r docversions.Ref, to string, after func(path string) pageFate) map[string]pageFate {
		pages, err := p.repo.Pages(ctx, r.Commit, p.rel)
		if err != nil {
			return nil
		}
		renamed, deleted, err := p.repo.Changes(ctx, r.Commit, to, p.rel)
		if err != nil {
			renamed, deleted = nil, nil
		}
		fates := make(map[string]pageFate, len(pages))
		for _, page := range pages {
			if deleted[page] {
				fates[page] = settle(page, true)
				continue
			}
			cur := page
			if next, ok := renamed[page]; ok {
				cur = next
			}
			fates[page] = after(cur)
		}
		return fates
	}
	record := func(name string, fates map[string]pageFate) {
		moved := make(map[string]string)
		edit := make(map[string]string)
		for page, f := range fates {
			switch {
			case f.now == "":
				edit[page] = ""
				if f.last != page {
					moved[page] = f.last
				}
			case f.now != page:
				moved[page] = f.now
				edit[page] = f.now
			}
		}
		if len(moved) > 0 {
			moves[name] = moved
		}
		edits[name] = edit
	}
	atWorktree := func(path string) pageFate { return settle(path, false) }

	var newer map[string]pageFate // fates of the next newer tag's pages
	newerCommit := ""             // and its commit; "" is the working tree
	for _, r := range p.refs {
		if !r.IsTag {
			record(r.Name, resolve(r, "", atWorktree))
			continue
		}
		after := atWorktree
		if newer != nil {
			prev := newer
			after = func(path string) pageFate {
				if f, ok := prev[path]; ok {
					return f
				}
				return atWorktree(path)
			}
		}
		fates := resolve(r, newerCommit, after)
		record(r.Name, fates)
		newer, newerCommit = fates, r.Commit
	}
	return moves, edits
}

// newestTag returns the newest published tag that is a release, not a
// prerelease.
func (p *versionPlan) newestTag() (docversions.Ref, bool) {
	for _, r := range p.refs {
		if r.IsTag && !docversions.Prerelease(r.Name) {
			return r, true
		}
	}
	return docversions.Ref{}, false
}

// pageAliases returns the aliases the pages under sourceDir list in
// their frontmatter, mapping each alias, as a source path, to the page
// that lists it.
func pageAliases(sourceDir string, files []markdownFile) map[string]string {
	aliases := make(map[string]string)
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(sourceDir, f.RelPath))
		if err != nil {
			continue
		}
		doc, err := parseFrontmatter(string(content))
		if err != nil {
			continue
		}
		for _, a := range docversions.Aliases(doc.Frontmatter) {
			aliases[docversions.AliasSource(a)] = filepath.ToSlash(f.RelPath)
		}
	}
	return aliases
}

// pageKey returns the key the manifest uses for the source page src.
func (l pageLayout) pageKey(src string) string {
	src = normalizeSourcePath(src)
	if src == normalizeSourcePath(l.index) || slices.Contains(indexNames, src) {
		return ""
	}
	l.index = ""
	return docversions.PageKey(l.url(src), l.ext)
}

// A versionedSite is a versioned site's plan and the manifest built
// from it.
type versionedSite struct {
	plan     *versionPlan
	manifest *docversions.Manifest
	// moves holds the source pages that moved, as [versionPlan.moves]
	// returns them.
	moves map[string]map[string]string
	// aliases maps each source path a page of the working tree lists
	// in its aliases to that page.
	aliases map[string]string
	// have holds the source paths of the working tree's pages.
	have     map[string]bool
	versions []GitVersion
	// edits holds each version's [pageVersion.edit], when the site
	// has edit links.
	edits map[string]map[string]string
}

// newVersionedSite plans the site's versions and builds their manifest.
// versionPath gives each version's root relative to the manifest.
func (site *preparedSite) newVersionedSite(ctx context.Context, versionPath func(name string) string) (*versionedSite, error) {
	cfg := site.config
	plan, err := site.planVersions(ctx)
	if err != nil {
		return nil, err
	}
	m := plan.manifest(versionPath)

	sourceDir, err := filepath.Abs(site.sourceDir())
	if err != nil {
		return nil, err
	}
	files, err := findMarkdownFiles(sourceDir, 100)
	if err != nil {
		return nil, fmt.Errorf("finding markdown files: %w", err)
	}
	have := make(map[string]bool, len(files))
	for _, f := range files {
		have[filepath.ToSlash(f.RelPath)] = true
	}
	aliases := pageAliases(sourceDir, files)
	moves, edits := plan.moves(ctx, aliases, have)
	layout := cfg.layout()
	for name, moved := range moves {
		keys := make(map[string]string, len(moved))
		for from, to := range moved {
			keys[layout.pageKey(from)] = layout.pageKey(to)
		}
		if m.Moves == nil {
			m.Moves = make(map[string]map[string]string)
		}
		m.Moves[name] = keys
	}

	vs := &versionedSite{plan: plan, manifest: m, moves: moves, aliases: aliases, have: have}
	for _, v := range m.Versions {
		vs.versions = append(vs.versions, GitVersion{Name: v.Name, Ref: v.Ref, IsTag: strings.HasPrefix(v.Ref, "refs/tags/"), Commit: v.Commit})
	}
	// Edit paths change as the working tree does, and a version's
	// cache key holds them, so they are kept only when used.
	if cfg.EditURL != "" {
		vs.edits = edits
	}
	return vs, nil
}

// pageVersion returns what the pages of version name know about it.
// manifest is the manifest's URL relative to the version root.
func (vs *versionedSite) pageVersion(name, manifest string) *pageVersion {
	return &pageVersion{name: name, manifest: manifest, versions: vs.versions, edit: vs.edits[name]}
}

// latestMoves returns the old source paths a reader may follow to the
// working tree, mapping each to the page there now: the paths of pages
// of any version that moved, the newest version's move winning, and
// the aliases pages list. Only paths that are no page now are kept.
func (vs *versionedSite) latestMoves() map[string]string {
	moves := make(map[string]string)
	add := func(from, to string) {
		if _, ok := moves[from]; !ok && !vs.have[from] && vs.have[to] {
			moves[from] = to
		}
	}
	for _, r := range vs.plan.refs {
		for from, to := range vs.moves[r.Name] {
			add(from, to)
		}
	}
	for from, to := range vs.aliases {
		add(from, to)
	}
	return moves
}

// generateVersionedHTML writes every published version of the site to
// its own directory under the output directory, with a manifest the
// pages' version switcher reads.
func (site *preparedSite) generateVersionedHTML(ctx context.Context, logger *slog.Logger) error {
	cfg := site.config
	vs, err := site.newVersionedSite(ctx, func(name string) string { return name + "/" })
	if err != nil {
		return err
	}
	plan, m, layout := vs.plan, vs.manifest, cfg.layout()
	if cfg.VersionDefault != "" {
		if _, ok := m.Lookup(cfg.VersionDefault); !ok {
			return fmt.Errorf("-version-default: no version %s", cfg.VersionDefault)
		}
		m.Default = cfg.VersionDefault
	}
	outDir := cfg.HTML
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	only, err := parseVersionsOnly(cfg.VersionsOnly, m)
	if err != nil {
		return err
	}
	aliasDirs, err := parseVersionAliases(cfg.VersionAlias, m, plan)
	if err != nil {
		return err
	}
	pv := func(name string) *pageVersion {
		return vs.pageVersion(name, "../"+docversions.ManifestName)
	}
	// Every version is rebuilt or restored below, each bringing the
	// shared files it uses, so none left by an earlier build is kept.
	sharedDir := filepath.Join(outDir, sharedDirName)
	if err := os.RemoveAll(sharedDir); err != nil {
		return err
	}

	var built []docversions.Version
	var errs []error
	for _, v := range m.Versions {
		dir := filepath.Join(outDir, v.Name)
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		var err error
		switch {
		case v.Archive:
			err = plan.copyArchive(ctx, v, m, dir)
		case v.Name == latestVersion:
			if only != nil && !only[v.Name] {
				logger.Info("skipping version", "version", v.Name)
				continue
			}
			logger.Info("building version", "version", v.Name, "ref", "working tree")
			latest := *site
			latest.config = versionConfig(cfg, v.Name)
			latest.config.HTML = dir
			latest.version = pv(v.Name)
			latest.shared = newSharedAssets(sharedDir)
			err = latest.generateStaticHTML(ctx, logger)
			if err == nil {
				err = writeMoveStubs(dir, layout, vs.latestMoves())
			}
		default:
			ref, _ := plan.ref(v.Name)
			var ok bool
			ok, err = site.buildRef(ctx, logger, plan, ref, pv(v.Name), dir, sharedDir, only == nil || only[v.Name])
			if err == nil && !ok {
				logger.Info("skipping version", "version", v.Name)
				continue
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("version %s: %w", v.Name, err))
			os.RemoveAll(dir)
			continue
		}
		if !v.Archive {
			v.Search = searchIndexName(dir)
		}
		built = append(built, v)
	}
	m.Versions = built
	if _, ok := m.Lookup(m.Default); !ok && len(built) > 0 {
		m.Default = built[0].Name
	}
	for name := range m.Moves {
		if _, ok := m.Lookup(name); !ok {
			delete(m.Moves, name)
		}
	}
	if len(m.Moves) == 0 {
		m.Moves = nil
	}

	for alias, target := range aliasDirs {
		if _, ok := m.Lookup(target); !ok {
			continue
		}
		dst := filepath.Join(outDir, alias)
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if err := copyTree(filepath.Join(outDir, target), dst); err != nil {
			return fmt.Errorf("version alias %s: %w", alias, err)
		}
		if m.Aliases == nil {
			m.Aliases = make(map[string]string)
		}
		m.Aliases[alias] = target
	}

	if err := m.WriteFile(filepath.Join(outDir, docversions.ManifestName)); err != nil {
		return err
	}
	if err := writeVersionsPage(filepath.Join(outDir, versionsPageDir, "index.html"), m, cfg.Title); err != nil {
		return err
	}
	if def, ok := m.Lookup(m.Default); ok {
		if err := writeRootStubs(outDir, def, m); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

// searchIndexName returns the name of the search index script in the
// built site at dir, or "" if it has none.
func searchIndexName(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "search-index.*.js"))
	if len(matches) != 1 {
		return ""
	}
	return filepath.Base(matches[0])
}

// versionConfig returns cfg as it is for building the version name:
// its pages are under the version's own directory of the site, while
// a relative -og-image still names a file relative to the site root.
func versionConfig(cfg Config, name string) Config {
	if cfg.SiteURL == "" {
		return cfg
	}
	root := strings.TrimSuffix(cfg.SiteURL, "/")
	if cfg.OGImage != "" {
		cfg.OGImage = absoluteURL(root+"/", cfg.OGImage)
	}
	cfg.SiteURL = root + "/" + name
	return cfg
}

// parseVersionsOnly parses -versions-only. A nil map builds every
// version.
func parseVersionsOnly(list string, m *docversions.Manifest) (map[string]bool, error) {
	if strings.TrimSpace(list) == "" {
		return nil, nil
	}
	only := make(map[string]bool)
	for name := range strings.SplitSeq(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := m.Lookup(name); !ok {
			return nil, fmt.Errorf("-versions-only: no version %s", name)
		}
		only[name] = true
	}
	return only, nil
}

// parseVersionAliases parses -version-alias, a comma-separated list of
// name=target pairs, into a map from alias directory to version. The
// target "newest-tag" names the newest published release.
func parseVersionAliases(list string, m *docversions.Manifest, plan *versionPlan) (map[string]string, error) {
	aliases := make(map[string]string)
	for pair := range strings.SplitSeq(list, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, target, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("-version-alias: %q is not name=target", pair)
		}
		if err := docversions.CheckName(name); err != nil {
			return nil, fmt.Errorf("-version-alias: %w", err)
		}
		if reservedName(name) {
			return nil, fmt.Errorf("-version-alias: %s is a reserved name", name)
		}
		if _, clash := m.Lookup(name); clash {
			return nil, fmt.Errorf("-version-alias: %s is already a version", name)
		}
		if target == "newest-tag" {
			r, ok := plan.newestTag()
			if !ok {
				continue // nothing released yet
			}
			target = r.Name
		}
		if _, ok := m.Lookup(target); !ok {
			return nil, fmt.Errorf("-version-alias: no version %s", target)
		}
		aliases[name] = target
	}
	return aliases, nil
}

// newSharedAssets returns where a version writes the files it shares
// with the other versions: sharedDir, one directory above its root.
func newSharedAssets(sharedDir string) *sharedAssets {
	return &sharedAssets{dir: sharedDir, prefix: "../" + sharedDirName + "/"}
}

// buildRef builds the site as of ref into dir, and the files it shares
// with other versions into sharedDir. A build is taken from the cache
// when one with the same inputs is there; otherwise, unless build is
// false, the ref is checked out into a worktree and built from it. It
// reports whether dir was written.
func (site *preparedSite) buildRef(ctx context.Context, logger *slog.Logger, plan *versionPlan, ref docversions.Ref, pv *pageVersion, dir, sharedDir string, build bool) (bool, error) {
	cfg := site.config
	var cached string
	if cfg.VersionCache != "" {
		key, err := site.versionCacheKey(ref, pv)
		if err != nil {
			return false, err
		}
		cached = filepath.Join(cfg.VersionCache, "builds", key)
		if info, err := os.Stat(cached); err == nil && info.IsDir() {
			logger.Info("using cached version", "version", ref.Name, "commit", shortCommit(ref.Commit))
			return true, restoreCache(cached, dir, sharedDir)
		}
	}
	if !build {
		return false, nil
	}
	logger.Info("building version", "version", ref.Name, "commit", shortCommit(ref.Commit))

	wtParent := ""
	if cfg.VersionCache != "" {
		wtParent = filepath.Join(cfg.VersionCache, "worktrees")
		if err := os.MkdirAll(wtParent, 0o755); err != nil {
			return false, err
		}
	}
	tmp, err := os.MkdirTemp(wtParent, "md2html-"+ref.Name+"-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	wt := filepath.Join(tmp, "src")
	if err := plan.repo.AddWorktree(ctx, wt, ref.Commit); err != nil {
		return false, err
	}
	defer plan.repo.RemoveWorktree(context.WithoutCancel(ctx), wt)

	cfg = versionConfig(cfg, pv.name)
	cfg.HTML = dir
	vsite, err := site.refSite(cfg, plan, wt, pv, logger)
	if errors.Is(err, errNoSource) {
		logger.Warn("version has no source directory", "version", ref.Name, "dir", plan.rel)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	vsite.shared = newSharedAssets(sharedDir)
	if err := vsite.generateStaticHTML(ctx, logger); err != nil {
		return false, err
	}
	if cached != "" {
		if err := storeCache(cached, dir, vsite.shared); err != nil {
			logger.Warn("caching version", "version", ref.Name, "error", err)
		}
	}
	return true, nil
}

// errNoSource reports that a version has no source directory: the
// docs were added after it was released.
var errNoSource = errors.New("no source directory")

// refSite returns the site cfg describes, read from the worktree wt of
// one of the plan's refs instead of the working tree. The source and
// the include root are read at that ref; the chrome (templates, CSS,
// components, icons) is shared with the working tree.
func (site *preparedSite) refSite(cfg Config, plan *versionPlan, wt string, pv *pageVersion, logger *slog.Logger) (*preparedSite, error) {
	cfg.Chdir = site.base
	cfg.Source = filepath.Join(wt, filepath.FromSlash(plan.rel))
	cfg.IncludeRoot = plan.remap(cfg.IncludeRoot, wt)
	cfg.Versions = false
	if info, err := os.Stat(cfg.Source); err != nil || !info.IsDir() {
		return nil, errNoSource
	}
	vsite, err := prepareSite(cfg, logger)
	if err != nil {
		return nil, err
	}
	if err := vsite.prepareIncludes(cfg.Source, cfg.Source); err != nil {
		return nil, err
	}
	if err := vsite.prepareFavicon(cfg.Source, logger); err != nil {
		return nil, err
	}
	vsite.version = pv
	return vsite, nil
}

// remap returns the path p, if it is inside the repository, at the same
// place inside the worktree wt. Other paths are returned unchanged.
func (p *versionPlan) remap(name, wt string) string {
	if name == "" {
		return ""
	}
	rel, err := docversions.RelPath(p.top, name)
	if err != nil {
		return name
	}
	return filepath.Join(wt, filepath.FromSlash(rel))
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

// versionCacheKey returns the key a build of ref is cached under: a
// hash of the commit, where its pages are edited, the md2html binary,
// the files that make up the site's chrome, and the configuration.
func (site *preparedSite) versionCacheKey(ref docversions.Ref, pv *pageVersion) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "md2html version cache 2\x00%s\x00%s\x00", ref.Commit, pv.name)
	// Edit links name pages as they are in the working tree, which
	// changes without the version changing.
	if err := json.NewEncoder(h).Encode(pv.edit); err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("version cache: %w", err)
	}
	if err := hashTree(h, exe); err != nil {
		return "", fmt.Errorf("version cache: %w", err)
	}
	cfg := site.config
	for _, p := range []string{cfg.TemplateDir, cfg.CSS, cfg.Components, cfg.Icons, cfg.DataJSON, cfg.JSONSpec, cfg.Favicon} {
		fmt.Fprintf(h, "%s\x00", p)
		if p != "" {
			if err := hashTree(h, p); err != nil {
				return "", fmt.Errorf("version cache: %w", err)
			}
		}
	}
	// Paths and modes that differ between builds without changing
	// what they write stay out of the key.
	cfg.HTML, cfg.VersionsOnly, cfg.VersionCache, cfg.VersionAlias = "", "", "", ""
	cfg.Verbose, cfg.Open, cfg.Vet = false, false, false
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))[:32], nil
}

// hashTree writes the names and contents of the files at root, a file
// or directory, to w.
func hashTree(w io.Writer, root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		fmt.Fprintf(w, "%s\x00", filepath.ToSlash(rel))
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}

// storeCache copies the finished build in dir, and the shared files it
// uses, into the cache at cached, replacing it whole so that a reader
// never sees part of one. The build is kept in site/ and the shared
// files in shared/.
func storeCache(cached, dir string, shared *sharedAssets) error {
	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(cached), ".tmp-")
	if err != nil {
		return err
	}
	err = copyTree(dir, filepath.Join(tmp, "site"))
	for _, f := range shared.files {
		if err != nil {
			break
		}
		err = copyFile(filepath.Join(shared.dir, filepath.FromSlash(f)), filepath.Join(tmp, "shared", filepath.FromSlash(f)))
	}
	if err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, cached); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	return nil
}

// restoreCache copies the build cached by storeCache into dir, and its
// shared files into sharedDir.
func restoreCache(cached, dir, sharedDir string) error {
	if err := copyTree(filepath.Join(cached, "site"), dir); err != nil {
		return err
	}
	shared := filepath.Join(cached, "shared")
	if _, err := os.Stat(shared); err != nil {
		return nil // the version shares nothing
	}
	return copyTree(shared, sharedDir)
}

// copyTree copies the directory src to dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(p, out)
	})
}

// copyArchive copies the archived version v into dir, adding a banner
// that links to the default version and the list of versions.
func (p *versionPlan) copyArchive(ctx context.Context, v docversions.Version, m *docversions.Manifest, dir string) error {
	var src string
	for _, a := range p.config.Archive {
		if a.Name == v.Name {
			src = a.Path
		}
	}
	if !strings.Contains(src, "://") && !filepath.IsAbs(src) {
		src = filepath.Join(p.configDir, src)
	}
	def, _ := m.Lookup(m.Default)
	return docversions.CopyArchive(ctx, src, dir, func(rel string) []byte {
		root := strings.Repeat("../", strings.Count(rel, "/")+1)
		return []byte(versionBanner(m.Banner.Old, v.Label, def.Label, root+def.Path, root+versionsPageDir+"/"))
	})
}

// versionBanner returns the banner HTML written into archived pages.
// text is the banner template; latestHref and listHref link to the
// default version and the list of versions.
func versionBanner(text, label, latestLabel, latestHref, listHref string) string {
	link := `<a href="` + html.EscapeString(latestHref) + `">` + html.EscapeString(latestLabel) + `</a>`
	body := html.EscapeString(text)
	body = strings.ReplaceAll(body, "{version}", html.EscapeString(label))
	body = strings.ReplaceAll(body, "{latest}", link)
	return `<div class="md2html-version-banner" role="note" style="padding:8px 16px;background:#fff8c5;color:#3b2300;border-bottom:1px solid #d4a72c;font:14px/1.5 system-ui,sans-serif">` +
		body + ` <a href="` + html.EscapeString(listHref) + `">All versions</a></div>`
}

// writeMoveStubs writes, into the latest version at dir, a redirect at
// the old path of every page that moved since the newest release, so
// that links into the latest docs keep working when a page moves.
func writeMoveStubs(dir string, layout pageLayout, moved map[string]string) error {
	layout.index = ""
	for from, to := range moved {
		stub := layout.file(from)
		if stub == "" || layout.file(to) == stub {
			continue
		}
		if err := writeRedirect(filepath.Join(dir, filepath.FromSlash(stub)), relativeURL(stub, layout.url(to))); err != nil {
			return err
		}
	}
	return nil
}

// writeRootStubs writes, at the site root, a redirect to each page of
// the default version def, so that the URLs the site had before it was
// versioned, and the root itself, open the default version. A file
// already at a stub's path, such as a page of an earlier build that was
// not versioned or a stub for an earlier default, is replaced.
func writeRootStubs(outDir string, def docversions.Version, m *docversions.Manifest) error {
	reserved := make(map[string]bool)
	for _, v := range m.Versions {
		reserved[v.Name] = true
	}
	for a := range m.Aliases {
		reserved[a] = true
	}
	src := filepath.Join(outDir, def.Name)
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		first, _, _ := strings.Cut(rel, "/")
		if reserved[first] || reservedName(first) {
			return nil
		}
		target := def.Path + rel
		if path.Base(rel) == "index.html" {
			target = strings.TrimSuffix(target, "index.html")
		}
		stub := filepath.Join(outDir, filepath.FromSlash(rel))
		if info, err := os.Lstat(stub); err == nil && info.Mode().IsRegular() {
			if err := os.Remove(stub); err != nil {
				return err
			}
		}
		return writeRedirect(stub, relativeURL(rel, target))
	})
}

// relativeURL returns the URL of target, relative to the site root, as
// written in the file at from, also relative to the site root.
func relativeURL(from, target string) string {
	dir := path.Dir(from)
	if dir == "." {
		if target == "" {
			return "./"
		}
		return target
	}
	up := strings.Repeat("../", strings.Count(dir, "/")+1)
	rel := up + target
	// Shorten a target inside from's own directory.
	if t, ok := strings.CutPrefix(target, dir+"/"); ok {
		rel = t
		if rel == "" {
			rel = "./"
		}
	}
	return rel
}

// writeRedirect writes a page at file that sends the reader to target.
// An existing file is left alone.
func writeRedirect(file, target string) error {
	if _, err := os.Stat(file); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	attr := html.EscapeString(target)
	js, err := json.Marshal(target)
	if err != nil {
		return err
	}
	page := `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Redirecting</title>
<meta name="robots" content="noindex">
<link rel="canonical" href="` + attr + `">
<meta http-equiv="refresh" content="0; url=` + attr + `">
<script>location.replace(` + string(js) + ` + location.search + location.hash)</script>
</head>
<body><p>This page has moved to <a href="` + attr + `">` + attr + `</a>.</p></body>
</html>
`
	return os.WriteFile(file, []byte(page), 0o644)
}

// writeVersionsPage writes the list of versions, which every page links
// to so that a reader without JavaScript can still change version.
func writeVersionsPage(file string, m *docversions.Manifest, title string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, versionsPage(m, title), 0o644)
}

// versionsPage returns the list of versions as an HTML page served one
// directory below the manifest.
func versionsPage(m *docversions.Manifest, title string) []byte {
	if title == "" || title == defaultTitle {
		title = "Documentation"
	}
	var b strings.Builder
	b.WriteString(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Versions · ` + html.EscapeString(title) + `</title>
<style>
:root{color-scheme:light dark;--fg:#1f2328;--bg:#fff;--muted:#59636e;--accent:#0969da}
@media (prefers-color-scheme:dark){:root{--fg:#e6edf3;--bg:#0d1117;--muted:#9198a1;--accent:#4493f8}}
html,body{background:var(--bg);color:var(--fg)}
body{font:16px/1.6 system-ui,sans-serif;max-width:40rem;margin:0 auto;padding:2rem 16px}
h1{color:var(--fg)}a{color:var(--accent)}li{margin:.25rem 0}.state{color:var(--muted);font-size:.875rem}
</style>
</head>
<body>
<h1>Versions</h1>
<ul>
`)
	for _, v := range m.Versions {
		state := v.State
		if v.Name == m.Default {
			state += ", default"
		}
		fmt.Fprintf(&b, "<li><a href=\"../%s\">%s</a> <span class=\"state\">(%s)</span></li>\n",
			html.EscapeString(v.Path), html.EscapeString(v.Label), html.EscapeString(state))
	}
	b.WriteString("</ul>\n</body>\n</html>\n")
	return []byte(b.String())
}
