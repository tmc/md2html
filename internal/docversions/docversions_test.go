package docversions

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseRenames(t *testing.T) {
	out := []byte("R100\tdocs/agents/agents.md\tdocs/agents.md\n" +
		"R087\tdocs/a.md\tdocs/b.mdx\n" +
		"R100\tdocs/img.png\tdocs/img2.png\n" +
		"R100\tother/x.md\tdocs/x.md\n" +
		"M\tdocs/c.md\n" +
		"D\tdocs/d.md\n")
	got := parseRenames(out, "docs")
	want := map[string]string{"agents/agents.md": "agents.md", "a.md": "b.mdx"}
	if !maps.Equal(got, want) {
		t.Errorf("parseRenames = %v, want %v", got, want)
	}
}

func TestPageKey(t *testing.T) {
	tests := []struct {
		url, ext, want string
	}{
		{"guide.html", ".html", "guide"},
		{"guide/", ".html", "guide"},
		{"guide/index.html", ".html", "guide"},
		{"index.html", ".html", ""},
		{"./", ".html", ""},
		{"", "", ""},
		{"a/b", "", "a/b"},
		{"a/b.html#x", ".html", "a/b"},
		{"/a/b/?q=1", ".html", "a/b"},
	}
	for _, tt := range tests {
		if got := PageKey(tt.url, tt.ext); got != tt.want {
			t.Errorf("PageKey(%q, %q) = %q, want %q", tt.url, tt.ext, got, tt.want)
		}
	}
}

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "versions.yaml")
	cfg, err := LoadConfig(file)
	if err != nil || !cfg.Publishes("v1.0.0") {
		t.Fatalf("missing config = %+v, %v; want an empty one publishing everything", cfg, err)
	}
	data := `include: ["v0.1.*", "v0.2.*"]
exclude: ["v0.1.17-rc*"]
collapse: minor
labels:
  v0.2.0: "v0.2.0 (LTS)"
archive:
  - name: v0.1.16
    label: v0.1.16 and earlier
    path: archive/v0.1.16
banner:
  dev: "unreleased"
`
	if err := os.WriteFile(file, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{
		"v0.1.17":     true,
		"v0.1.17-rc1": false,
		"v0.2.0":      true,
		"v0.3.0":      false,
	} {
		if got := cfg.Publishes(name); got != want {
			t.Errorf("Publishes(%q) = %v, want %v", name, got, want)
		}
	}
	if got := cfg.Label("v0.2.0"); got != "v0.2.0 (LTS)" {
		t.Errorf("Label = %q", got)
	}
	hidden := cfg.Hidden([]string{"v0.2.1", "v0.2.0", "v0.1.18", "v0.1.17"})
	if want := map[string]bool{"v0.2.0": true, "v0.1.17": true}; !maps.Equal(hidden, want) {
		t.Errorf("Hidden = %v, want %v", hidden, want)
	}
	if b := cfg.BannerText(); b.Dev != "unreleased" || b.Old != DefaultBanner.Old {
		t.Errorf("BannerText = %+v", b)
	}
	if len(cfg.Archive) != 1 || cfg.Archive[0].Path != "archive/v0.1.16" {
		t.Errorf("Archive = %+v", cfg.Archive)
	}

	for _, tt := range []struct {
		config string
		want   map[string]bool
	}{
		{"since: v0.1.17\n", map[string]bool{
			"v0.1.16":       false,
			"v0.1.17-rc1":   false,
			"v0.1.17":       true,
			"v0.1.18-pre.0": false,
			"v0.1.100":      true,
			"v1.0.0":        true,
			"v1.0.0-rc.1":   false,
			"release-x":     false,
		}},
		{"since: v0.1.17\ninclude: ['v*']\n", map[string]bool{
			"v0.1.18":       true,
			"v0.1.18-pre.0": false,
		}},
		{"since: v0.1.17\ninclude: ['v0.*', 'v1.0.0-rc.*']\n", map[string]bool{
			"v0.1.18":       true,
			"v0.1.18-pre.0": false,
			"v1.0.0-rc.1":   true,
			"v1.0.0-beta.1": false,
			"v1.0.0":        false,
		}},
		{"since: v1.0.0-rc.1\n", map[string]bool{
			"v1.0.0-beta.1": false,
			"v1.0.0-rc.1":   true,
			"v1.0.0-rc.2":   true,
			"v1.0.0":        true,
		}},
		{"exclude: ['*-rc*']\n", map[string]bool{
			"v0.1.18-pre.0": true,
			"v1.0.0-rc.1":   false,
		}},
	} {
		if err := os.WriteFile(file, []byte(tt.config), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(file)
		if err != nil {
			t.Fatal(err)
		}
		for name, want := range tt.want {
			if got := cfg.Publishes(name); got != want {
				t.Errorf("%q: Publishes(%q) = %v, want %v", tt.config, name, got, want)
			}
		}
	}

	for _, bad := range []string{"since: latest\n", "collapse: patch\n", "archive:\n  - name: x\n", "archive:\n  - name: a/b\n    path: p\n", "include: ['[']\n"} {
		if err := os.WriteFile(file, []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(file); err == nil {
			t.Errorf("LoadConfig(%q) succeeded, want error", bad)
		}
	}
}

func TestInjectBody(t *testing.T) {
	tests := []struct{ page, want string }{
		{`<html><BODY class="x"><p>hi</p></BODY></html>`, `<html><BODY class="x"><b>!</b><p>hi</p></BODY></html>`},
		{`<p>hi</p>`, `<b>!</b><p>hi</p>`},
	}
	for _, tt := range tests {
		if got := string(injectBody([]byte(tt.page), []byte("<b>!</b>"))); got != tt.want {
			t.Errorf("injectBody(%q) = %q, want %q", tt.page, got, tt.want)
		}
	}
}

func TestCopyArchive(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"a/page.html": "<body>x</body>", "style.css": "body{}"} {
		if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := CopyArchive(context.Background(), src, dst, func(rel string) []byte { return []byte("[" + rel + "]") })
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "a", "page.html")); string(got) != "<body>[a/page.html]x</body>" {
		t.Errorf("page = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "style.css")); string(got) != "body{}" {
		t.Errorf("css = %q", got)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "t@example.com")
	git(t, dir, "config", "user.name", "T")
	write("docs/agents/agents.md", "# Agents\n\nA long enough body that rename detection matches it.\n")
	write("docs/index.md", "# Home\n")
	write("README.md", "# Repo\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "one")
	git(t, dir, "tag", "v0.1.9")
	git(t, dir, "tag", "-a", "-m", "annotated", "v0.1.10")
	git(t, dir, "mv", "docs/agents/agents.md", "docs/agents.md")
	git(t, dir, "commit", "-q", "-m", "two")
	git(t, dir, "tag", "v0.2.0")
	git(t, dir, "tag", "other")

	ctx := context.Background()
	r := Repo{Dir: filepath.Join(dir, "docs")}
	if !r.IsRepo(ctx) {
		t.Fatal("IsRepo = false")
	}
	tags, err := r.Tags(ctx, "v*")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tag := range tags {
		names = append(names, tag.Name)
		if len(tag.Commit) < 40 {
			t.Errorf("tag %s commit = %q, want a full hash", tag.Name, tag.Commit)
		}
	}
	if want := []string{"v0.2.0", "v0.1.10", "v0.1.9"}; !slices.Equal(names, want) {
		t.Errorf("Tags = %v, want %v", names, want)
	}
	if tags[1].Commit != tags[2].Commit {
		t.Errorf("annotated tag commit %s != %s", tags[1].Commit, tags[2].Commit)
	}

	moves, err := r.Renames(ctx, "v0.1.10", "v0.2.0", "docs")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"agents/agents.md": "agents.md"}; !maps.Equal(moves, want) {
		t.Errorf("Renames = %v, want %v", moves, want)
	}
	pages, err := r.Pages(ctx, "v0.1.9", "docs")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"agents/agents.md", "index.md"}; !slices.Equal(pages, want) {
		t.Errorf("Pages = %v, want %v", pages, want)
	}

	wt := filepath.Join(t.TempDir(), "wt")
	if err := r.AddWorktree(ctx, wt, tags[2].Commit); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wt, "docs", "agents", "agents.md")); err != nil {
		t.Errorf("worktree is missing the tagged page: %v", err)
	}
	if err := r.RemoveWorktree(ctx, wt); err != nil {
		t.Fatal(err)
	}
}

func TestAliases(t *testing.T) {
	tests := []struct {
		name string
		fm   map[string]any
		want []string
	}{
		{"none", map[string]any{"title": "x"}, nil},
		{"one", map[string]any{"aliases": "old"}, []string{"old"}},
		{"list", map[string]any{"aliases": []any{"a", "", 3, "b"}}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		if got := Aliases(tt.fm); !slices.Equal(got, tt.want) {
			t.Errorf("%s: Aliases = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestAliasSource(t *testing.T) {
	tests := []struct{ alias, want string }{
		{"modules/agents/agents", "modules/agents/agents.md"},
		{"/old/", "old.md"},
		{"guide.mdx", "guide.mdx"},
		{"a/../b", "b.md"},
		{" spaced/page.markdown ", "spaced/page.markdown"},
	}
	for _, tt := range tests {
		if got := AliasSource(tt.alias); got != tt.want {
			t.Errorf("AliasSource(%q) = %q, want %q", tt.alias, got, tt.want)
		}
	}
}

func TestRelPath(t *testing.T) {
	top := t.TempDir()
	if err := os.MkdirAll(filepath.Join(top, "docs", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(top, link); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, dir, want string
		ok              bool
	}{
		{"root", top, ".", true},
		{"inside", filepath.Join(top, "docs", "a"), "docs/a", true},
		{"through a symlink", filepath.Join(link, "docs"), "docs", true},
		{"outside", t.TempDir(), "", false},
	}
	for _, tt := range tests {
		got, err := RelPath(top, tt.dir)
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("%s: RelPath = %q, %v; want %q, ok=%v", tt.name, got, err, tt.want, tt.ok)
		}
	}
}
