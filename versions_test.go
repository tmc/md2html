package md2html

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tmc/md2html/internal/docversions"
)

// versionedRepo returns a repository whose docs directory has two
// tagged releases and unreleased changes on top:
//
//   - v0.1.9 has agents/agents.md and includes examples/hello.go;
//   - v0.1.10 changes the example;
//   - the working tree moves agents/agents.md to agents.md and adds
//     a page whose frontmatter names a removed page as an alias.
func versionedRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) { writeTestFile(t, dir, name, body) }
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	write("docs/index.md", "# Home\n\nSee [agents](agents/agents.md).\n")
	write("docs/agents/agents.md", "# Agents\n\nAgents call tools in a loop until the task is done.\n")
	write("docs/guide.md", "# Guide\n\n<Code src=\"/examples/hello.go\" />\n")
	write("docs/old.md", "# Old\n\nA page that later goes away.\n")
	write("docs/gone.md", "# Gone\n\nA page removed with nothing in its place.\n")
	write("examples/hello.go", "package main // v0.1.9\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "v0.1.9")
	runGit(t, dir, "tag", "v0.1.9")
	write("examples/hello.go", "package main // v0.1.10\n")
	runGit(t, dir, "commit", "-q", "-am", "v0.1.10")
	runGit(t, dir, "tag", "v0.1.10")
	runGit(t, dir, "mv", "docs/agents/agents.md", "docs/agents.md")
	runGit(t, dir, "rm", "-q", "docs/old.md", "docs/gone.md")
	write("docs/index.md", "# Home\n\nSee [agents](agents.md).\n")
	write("docs/new.md", "---\ntitle: New\naliases: [old]\n---\n\nReplaces the old page.\n")
	write("examples/hello.go", "package main // main\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "reorganize")
	return dir
}

// readManifest returns the versions.json manifest in the directory dir.
func readManifest(t *testing.T, dir string) *docversions.Manifest {
	t.Helper()
	m := new(docversions.Manifest)
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "versions.json"))), m); err != nil {
		t.Fatal(err)
	}
	return m
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestVersionedStaticBuild(t *testing.T) {
	repo := versionedRepo(t)
	out := filepath.Join(t.TempDir(), "out")
	cfg := Config{
		Chdir:          repo,
		Source:         "docs",
		HTML:           out,
		Title:          "Docs",
		IncludeRoot:    ".",
		Versions:       true,
		VersionPattern: "v*",
		VersionAlias:   "stable=newest-tag",
		SiteURL:        "https://example.com/docs",
		OGImage:        "../img/card.png",
		EditURL:        "https://example.com/edit/docs/{path}",
		Search:         true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, logger, io.Discard, nil); err != nil {
		t.Fatal(err)
	}

	m := readManifest(t, out)
	var names, states []string
	for _, v := range m.Versions {
		names = append(names, v.Name)
		states = append(states, v.State)
	}
	if got, want := strings.Join(names, " "), "latest v0.1.10 v0.1.9"; got != want {
		t.Errorf("versions = %s, want %s", got, want)
	}
	if got, want := strings.Join(states, " "), "dev stable old"; got != want {
		t.Errorf("states = %s, want %s", got, want)
	}
	for _, v := range m.Versions {
		if v.Search == "" {
			t.Errorf("version %s names no search index", v.Name)
		} else if _, err := os.Stat(filepath.Join(out, v.Path, v.Search)); err != nil {
			t.Errorf("version %s search index: %v", v.Name, err)
		}
	}
	if m.Default != "latest" || m.Aliases["stable"] != "v0.1.10" {
		t.Errorf("default = %q, aliases = %v", m.Default, m.Aliases)
	}
	if got := m.Moves["v0.1.10"]["agents/agents"]; got != "agents" {
		t.Errorf("moves[v0.1.10][agents/agents] = %q, want agents (moves = %v)", got, m.Moves)
	}
	if got := m.Moves["v0.1.10"]["old"]; got != "new" {
		t.Errorf("moves[v0.1.10][old] = %q, want new from the alias", got)
	}

	// Each version renders its own pages and its own includes.
	for _, tt := range []struct{ file, want string }{
		{"latest/guide.html", "// main"},
		{"v0.1.10/guide.html", "// v0.1.10"},
		{"v0.1.9/guide.html", "// v0.1.9"},
		{"stable/guide.html", "// v0.1.10"},
		{"v0.1.9/agents/agents.html", "Agents"},
		{"latest/agents.html", "Agents"},
	} {
		if got := readFile(t, filepath.Join(out, tt.file)); !strings.Contains(got, tt.want) {
			t.Errorf("%s does not contain %q", tt.file, tt.want)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "latest", "old.html")); err != nil {
		t.Errorf("latest is missing the redirect for the aliased page: %v", err)
	}
	if got := readFile(t, filepath.Join(out, "latest", "agents", "agents.html")); !strings.Contains(got, `url=../agents.html`) {
		t.Errorf("latest/agents/agents.html is not a redirect to the moved page:\n%s", got)
	}

	// The search scripts, the same in every version, are kept once.
	for _, pattern := range []string{"_shared/js/search.*.js", "_shared/js/minisearch.min.*.js"} {
		if matches, _ := filepath.Glob(filepath.Join(out, pattern)); len(matches) != 1 {
			t.Errorf("%s matches %q, want one file", pattern, matches)
		}
	}
	for _, v := range []string{"latest", "v0.1.10", "v0.1.9", "stable"} {
		if matches, _ := filepath.Glob(filepath.Join(out, v, "js", "search.*.js")); len(matches) != 0 {
			t.Errorf("%s keeps its own search script: %q", v, matches)
		}
	}

	// Pages carry the switcher, scoped to their version.
	page := readFile(t, filepath.Join(out, "v0.1.9", "agents", "agents.html"))
	for _, want := range []string{
		`data-versions="../../versions.json"`,
		`data-version="v0.1.9"`,
		`data-page="agents/agents"`,
		`href="../../versions/"`,
		`src="../../_shared/js/search.`,
		`data-root="../"`,
		`<link rel="canonical" href="https://example.com/docs/v0.1.9/agents/agents.html"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("v0.1.9/agents/agents.html does not contain %s", want)
		}
	}

	// A relative -og-image names a file relative to the site root, as
	// it does when the site is not versioned.
	for _, f := range []string{"latest/guide.html", "v0.1.9/guide.html"} {
		const want = `<meta property="og:image" content="https://example.com/img/card.png">`
		if got := readFile(t, filepath.Join(out, f)); !strings.Contains(got, want) {
			t.Errorf("%s does not contain %s", f, want)
		}
	}

	// Edit links name pages as they are now: an old version's page
	// where it moved to, and none for a page since removed.
	for f, want := range map[string]string{
		"latest/guide.html":         "docs/guide.md",
		"v0.1.9/guide.html":         "docs/guide.md",
		"v0.1.9/agents/agents.html": "docs/agents.md",
		"v0.1.9/old.html":           "docs/new.md",
		"v0.1.10/gone.html":         "",
	} {
		page := readFile(t, filepath.Join(out, f))
		_, link, ok := strings.Cut(page, `class="edit-link" href="https://example.com/edit/`)
		link, _, _ = strings.Cut(link, `"`)
		if want == "" && ok {
			t.Errorf("%s has an edit link to %s, want none", f, link)
		}
		if want != "" && link != want {
			t.Errorf("%s has an edit link to %q, want %q", f, link, want)
		}
	}

	// The root opens the default version, and the URLs the site had
	// before it was versioned still resolve.
	if got := readFile(t, filepath.Join(out, "index.html")); !strings.Contains(got, `url=latest/`) {
		t.Errorf("index.html does not redirect to latest/:\n%s", got)
	}
	if got := readFile(t, filepath.Join(out, "guide.html")); !strings.Contains(got, `url=latest/guide.html`) {
		t.Errorf("guide.html does not redirect to latest/guide.html:\n%s", got)
	}
	if got := readFile(t, filepath.Join(out, "versions", "index.html")); !strings.Contains(got, `href="../v0.1.9/"`) {
		t.Errorf("versions/index.html does not list v0.1.9:\n%s", got)
	}
}

func TestVersionedStaticBuildCache(t *testing.T) {
	repo := versionedRepo(t)
	cache := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	build := func(out, only string) {
		t.Helper()
		cfg := Config{
			Chdir:          repo,
			Source:         "docs",
			HTML:           out,
			IncludeRoot:    ".",
			Versions:       true,
			VersionPattern: "v*",
			VersionCache:   cache,
			VersionsOnly:   only,
			Search:         true,
		}
		if err := Run(context.Background(), cfg, logger, io.Discard, nil); err != nil {
			t.Fatal(err)
		}
	}
	first := filepath.Join(t.TempDir(), "out")
	build(first, "")
	builds, err := os.ReadDir(filepath.Join(cache, "builds"))
	if err != nil || len(builds) != 2 {
		t.Fatalf("cache holds %d builds (%v), want one per tag", len(builds), err)
	}

	// A subset build takes the other tags from the cache.
	second := filepath.Join(t.TempDir(), "out")
	build(second, "latest")
	shared, err := filepath.Glob(filepath.Join(first, "_shared", "js", "*.js"))
	if err != nil || len(shared) == 0 {
		t.Fatalf("no shared scripts in the full build (%v)", err)
	}
	for _, f := range shared {
		rel, err := filepath.Rel(first, f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(second, rel)); err != nil {
			t.Errorf("cached build is missing %s: %v", rel, err)
		}
	}
	for _, f := range []string{"latest/guide.html", "v0.1.9/guide.html", "v0.1.10/guide.html"} {
		if a, b := readFile(t, filepath.Join(first, f)), readFile(t, filepath.Join(second, f)); a != b {
			t.Errorf("%s differs between the full and the cached build", f)
		}
	}

	// Without the cache, versions not asked for are left out.
	third := filepath.Join(t.TempDir(), "out")
	cfg := Config{Chdir: repo, Source: "docs", HTML: third, IncludeRoot: ".", Versions: true, VersionPattern: "v*", VersionsOnly: "v0.1.9"}
	if err := Run(context.Background(), cfg, logger, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, third)
	if len(m.Versions) != 1 || m.Versions[0].Name != "v0.1.9" || m.Default != "v0.1.9" {
		t.Errorf("manifest = %+v, want only v0.1.9, as the default", m)
	}
}

func TestVersionedStaticBuildArchive(t *testing.T) {
	repo := versionedRepo(t)
	writeTestFile(t, repo, "archive/v0.0.1/docs/intro.html", "<html><body><p>old site</p></body></html>")
	writeTestFile(t, repo, "docs/versions.yaml", `exclude: ["v0.1.9"]
labels:
  latest: main
archive:
  - name: v0.0.1
    label: v0.0.1 and earlier
    path: ../archive/v0.0.1
banner:
  old: "Old docs for {version}; see {latest}."
`)
	out := filepath.Join(t.TempDir(), "out")
	cfg := Config{Chdir: repo, Source: "docs", HTML: out, IncludeRoot: ".", Versions: true, VersionPattern: "v*"}
	if err := Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, out)
	var names []string
	for _, v := range m.Versions {
		names = append(names, v.Name+"="+v.Label)
	}
	if got, want := strings.Join(names, ","), "latest=main,v0.1.10=v0.1.10,v0.0.1=v0.0.1 and earlier"; got != want {
		t.Errorf("versions = %s, want %s", got, want)
	}
	got := readFile(t, filepath.Join(out, "v0.0.1", "docs", "intro.html"))
	want := `<body><div class="md2html-version-banner"`
	if !strings.Contains(got, want) || !strings.Contains(got, `Old docs for v0.0.1 and earlier; see <a href="../../latest/">main</a>.`) || !strings.Contains(got, `<p>old site</p>`) {
		t.Errorf("archived page:\n%s", got)
	}
}

func TestRelativeURL(t *testing.T) {
	tests := []struct{ from, target, want string }{
		{"index.html", "latest/", "latest/"},
		{"guide.html", "latest/guide.html", "latest/guide.html"},
		{"a/b.html", "latest/a/b.html", "../latest/a/b.html"},
		{"a/b/c.html", "a/b/d.html", "d.html"},
		{"agents/agents.html", "agents.html", "../agents.html"},
		{"a/index.html", "a/", "./"},
	}
	for _, tt := range tests {
		if got := relativeURL(tt.from, tt.target); got != tt.want {
			t.Errorf("relativeURL(%q, %q) = %q, want %q", tt.from, tt.target, got, tt.want)
		}
	}
}

// TestVersionMoves checks that each version's moves lead to the page
// as it is in the working tree, however many releases it moved across
// and whichever versions are built.
func TestVersionMoves(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) { writeTestFile(t, dir, name, body) }
	git := func(args ...string) { runGit(t, dir, args...) }
	git("init", "-q")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	body := "# Page\n\n" + strings.Repeat("A line the page keeps from release to release.\n", 5)
	write("docs/index.md", "# Home\n")
	write("docs/a.md", body)
	write("docs/d.md", "# Dropped\n\nA page deleted in the next release.\n")
	git("add", ".")
	git("commit", "-q", "-m", "v0.1.0")
	git("tag", "v0.1.0")
	git("mv", "docs/a.md", "docs/b.md")
	git("rm", "-q", "docs/d.md")
	git("commit", "-q", "-m", "v0.2.0")
	git("tag", "v0.2.0")
	// Rewritten in place: by the working tree, a.md is too unlike the
	// page to be found as a rename of it directly.
	write("docs/b.md", "# Page\n\n"+strings.Repeat("Text written for the next release instead.\n", 5))
	// A new page where the deleted one was is not that page.
	write("docs/d.md", "# Different\n\n"+strings.Repeat("Another page that happens to reuse the name.\n", 5))
	git("add", ".")
	git("commit", "-q", "-m", "v0.3.0")
	git("tag", "v0.3.0")
	git("tag", "v0.4.0-rc.1")
	git("update-ref", "refs/remotes/origin/next", "HEAD")
	git("mv", "docs/b.md", "docs/c.md")
	git("mv", "docs/d.md", "docs/e.md")
	// An alias for a path no version had still redirects.
	write("docs/f.md", "---\naliases: [never]\n---\n\n# F\n")
	git("add", ".")
	git("commit", "-q", "-m", "move")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var out string
	build := func(only string) *docversions.Manifest {
		t.Helper()
		out = filepath.Join(t.TempDir(), "out")
		cfg := Config{
			Chdir:           dir,
			Source:          "docs",
			HTML:            out,
			Versions:        true,
			VersionPattern:  "v*",
			VersionBranches: true,
			VersionsOnly:    only,
			EditURL:         "https://example.com/edit/{path}",
		}
		if err := Run(context.Background(), cfg, logger, io.Discard, nil); err != nil {
			t.Fatal(err)
		}
		return readManifest(t, out)
	}

	m := build("")
	var got []string
	for _, v := range m.Versions {
		got = append(got, v.Name+":"+v.State)
	}
	// Newest first: the working tree, branches, then tags, of which the
	// newest release, not the newer release candidate, is stable.
	want := []string{"latest:dev", "next:dev", "v0.4.0-rc.1:dev", "v0.3.0:stable", "v0.2.0:old", "v0.1.0:old"}
	if !slices.Equal(got, want) {
		t.Errorf("versions = %v, want %v", got, want)
	}
	for v, want := range map[string]map[string]string{
		"v0.1.0":      {"a": "c"},
		"v0.2.0":      {"b": "c"},
		"v0.3.0":      {"b": "c", "d": "e"},
		"v0.4.0-rc.1": {"b": "c", "d": "e"},
		"next":        {"b": "c", "d": "e"},
	} {
		if got := m.Moves[v]; !maps.Equal(got, want) {
			t.Errorf("moves[%s] = %v, want %v", v, got, want)
		}
	}

	// The deleted page has no edit link; the page now at its old path
	// is no relation.
	if page := readFile(t, filepath.Join(out, "v0.1.0", "d.html")); strings.Contains(page, `class="edit-link"`) {
		t.Errorf("v0.1.0/d.html has an edit link, want none")
	}
	if page := readFile(t, filepath.Join(out, "v0.3.0", "d.html")); !strings.Contains(page, `href="https://example.com/edit/e.md"`) {
		t.Errorf("v0.3.0/d.html does not link to editing e.md")
	}
	// Latest redirects every old path a reader may follow: moves of any
	// version, and aliases.
	for stub, want := range map[string]string{"a.html": "c.html", "b.html": "c.html", "d.html": "e.html", "never.html": "f.html"} {
		if page := readFile(t, filepath.Join(out, "latest", stub)); !strings.Contains(page, "url="+want) {
			t.Errorf("latest/%s does not redirect to %s:\n%s", stub, want, page)
		}
	}

	// Building only the oldest release leaves its moves as they were.
	m = build("v0.1.0")
	if got, want := m.Moves["v0.1.0"], map[string]string{"a": "c"}; !maps.Equal(got, want) {
		t.Errorf("with -versions-only v0.1.0: moves[v0.1.0] = %v, want %v", got, want)
	}
}

func TestVersionedServer(t *testing.T) {
	repo := versionedRepo(t)
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		cfg := Config{
			Chdir:          repo,
			Source:         "docs",
			HTTP:           addr,
			Watch:          "false",
			IncludeRoot:    ".",
			Versions:       true,
			VersionPattern: "v*",
			Search:         true,
			Nav:            true,
			EditURL:        "https://example.com/edit/docs/{path}",
			Index:          "docs/index.md",
			VersionDefault: "v0.1.10",
		}
		done <- Run(ctx, cfg, discardLogger(), io.Discard, nil)
	}()
	waitServing(t, addr)

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := http.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(body)
	}
	for _, tt := range []struct {
		path string
		code int
		want string
	}{
		{"/guide", 200, "// main"},
		// The -index page is read from the version's own tree.
		{"/", 200, `See <a href="docs/agents">`},
		{"/v/v0.1.9/", 200, `See <a href="docs/agents/agents">`},
		{"/guide", 200, `data-versions="./versions.json"`},
		{"/v/v0.1.9/guide", 200, "// v0.1.9"},
		{"/v/v0.1.10/guide", 200, "// v0.1.10"},
		{"/v/v0.1.9/guide", 200, `../../versions.json"`},
		{"/v/v0.1.9/agents/agents", 200, "Agents"},
		{"/guide", 200, `href="https://example.com/edit/docs/guide.md"`},
		{"/v/v0.1.9/agents/agents", 200, `href="https://example.com/edit/docs/agents.md"`},
		{"/v/v0.1.9/old", 200, `href="https://example.com/edit/docs/new.md"`},
		{"/v/v0.1.9/search-index.js", 200, "Old"},
		{"/search-index.js", 200, "Replaces the old page"},
		{"/versions.json", 200, `"path": "v/v0.1.9/"`},
		{"/versions.json", 200, `"search": "search-index.js"`},
		{"/versions.json", 200, `"default": "latest"`}, // not -version-default
		{"/versions/", 200, `href="../v/v0.1.10/"`},
		{"/v/v9.9.9/guide", 404, ""},
		{"/old", 200, "Replaces the old page"}, // through the redirect an alias makes
	} {
		code, body := get(tt.path)
		if code != tt.code || !strings.Contains(body, tt.want) {
			t.Errorf("GET %s = %d, want %d containing %q", tt.path, code, tt.code, tt.want)
		}
	}
	// A page removed since its version has no edit link.
	if code, body := get("/v/v0.1.10/gone"); code != 200 || strings.Contains(body, `class="edit-link"`) {
		t.Errorf("GET /v/v0.1.10/gone = %d, want 200 with no edit link", code)
	}
	// A page moved since the newest release redirects to where it went.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get("http://" + addr + "/agents/agents")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode != http.StatusMovedPermanently || loc != "../agents" {
		t.Errorf("GET /agents/agents = %d, Location %q; want 301 to ../agents", resp.StatusCode, loc)
	}
	if code, body := get("/v/v0.1.9/search-index.js"); code == 200 && strings.Contains(body, "Replaces the old page") {
		t.Error("v0.1.9's search index lists a page from the working tree")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", repo, "worktree", "list").Output()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(out), "\n"); n != 1 {
		t.Errorf("worktrees left after the server stopped:\n%s", out)
	}
}
