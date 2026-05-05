package tabs

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
)

func render(t *testing.T, src string) string {
	t.Helper()
	md := goldmark.New(goldmark.WithExtensions(Extender{}))
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return buf.String()
}

func TestKebab(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Ubuntu", "ubuntu"},
		{"Arch Linux", "arch-linux"},
		{"  Debian 12  ", "debian-12"},
		{"macOS (Intel)", "macos-intel"},
		{"---", ""},
		{"Node.js", "nodejs"},
	}
	for _, tc := range tests {
		if got := kebab(tc.in); got != tc.want {
			t.Errorf("kebab(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenderBasic(t *testing.T) {
	src := "::: tabs install\n::: tab Ubuntu\nhello ubuntu\n:::\n::: tab Debian\nhello debian\n:::\n:::\n"
	out := render(t, src)

	for _, want := range []string{
		`data-tab-group="install"`,
		`role="tablist"`,
		`id="install-ubuntu"`,
		`id="install-debian"`,
		`aria-controls="install-ubuntu"`,
		`aria-labelledby="install-ubuntu-tab"`,
		`data-tab-slug="ubuntu"`,
		`>Ubuntu<`,
		`>Debian<`,
		`hello ubuntu`,
		`hello debian`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}

	// Progressive enhancement: all panels emitted visible so no-JS
	// readers see every tab's content stacked. The tabs-script
	// collapses non-selected panels on DOMContentLoaded.
	if strings.Contains(out, `hidden>`) || strings.Contains(out, ` hidden `) {
		t.Errorf("no panel should be rendered with the hidden attribute, got:\n%s", out)
	}
}

func TestSlugCollisionDedupe(t *testing.T) {
	src := "::: tabs x\n::: tab Same\nA\n:::\n::: tab Same\nB\n:::\n:::\n"
	out := render(t, src)
	if !strings.Contains(out, `id="x-same"`) {
		t.Errorf("first slug missing\n%s", out)
	}
	if !strings.Contains(out, `id="x-same-2"`) {
		t.Errorf("collision slug missing\n%s", out)
	}
}

func TestSlugPerGroup(t *testing.T) {
	// Slug counters are scoped per group: "Ubuntu" in two different
	// groups should not collide.
	src := "::: tabs install\n::: tab Ubuntu\nA\n:::\n:::\n\n::: tabs uninstall\n::: tab Ubuntu\nB\n:::\n:::\n"
	out := render(t, src)
	if !strings.Contains(out, `id="install-ubuntu"`) {
		t.Errorf("install-ubuntu missing\n%s", out)
	}
	if !strings.Contains(out, `id="uninstall-ubuntu"`) {
		t.Errorf("uninstall-ubuntu missing\n%s", out)
	}
	if strings.Contains(out, `id="install-ubuntu-2"`) {
		t.Errorf("slug counter leaked between groups\n%s", out)
	}
}

func TestMissingGroupID(t *testing.T) {
	// "::: tabs" with no id should not produce tab markup.
	src := "::: tabs\n::: tab Ubuntu\ncontent\n:::\n:::\n"
	out := render(t, src)
	if strings.Contains(out, `class="md-tabs"`) {
		t.Errorf("expected no tab markup for group without id, got:\n%s", out)
	}
}

func TestTabOutsideGroup(t *testing.T) {
	src := "::: tab Ubuntu\ncontent\n:::\n"
	out := render(t, src)
	if strings.Contains(out, `md-tab-panel`) {
		t.Errorf("expected no panel for orphan tab, got:\n%s", out)
	}
}

func TestCodeBlockInsideTab(t *testing.T) {
	src := "::: tabs cmd\n::: tab Bash\n```bash\necho hi\n```\n:::\n:::\n"
	out := render(t, src)
	if !strings.Contains(out, `<pre>`) && !strings.Contains(out, "<code") {
		t.Errorf("fenced code block inside tab did not render:\n%s", out)
	}
	if !strings.Contains(out, `id="cmd-bash"`) {
		t.Errorf("tab panel missing:\n%s", out)
	}
}

func TestLabelWithSpaces(t *testing.T) {
	src := "::: tabs os\n::: tab Arch Linux\ncontent\n:::\n:::\n"
	out := render(t, src)
	if !strings.Contains(out, `id="os-arch-linux"`) {
		t.Errorf("multi-word label did not kebab correctly:\n%s", out)
	}
	if !strings.Contains(out, `>Arch Linux<`) {
		t.Errorf("display label lost:\n%s", out)
	}
}

func TestMultipleGroupsOnPage(t *testing.T) {
	src := "::: tabs a\n::: tab One\nfirst\n:::\n:::\n\n::: tabs b\n::: tab Two\nsecond\n:::\n:::\n"
	out := render(t, src)
	if strings.Count(out, `role="tablist"`) != 2 {
		t.Errorf("expected 2 tablists, got:\n%s", out)
	}
}
