package md2html

import (
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeIconSet lays out a directory of SVG files named the way pages name
// icons in frontmatter.
func writeIconSet(t *testing.T, icons map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range icons {
		if err := os.WriteFile(filepath.Join(dir, name+".svg"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const testRocketSVG = `<!-- @license example - ISC -->
<svg xmlns="http://www.w3.org/2000/svg"
  width="24"
  height="24"
  viewBox="0 0 24 24"
  stroke="currentColor">
  <path d="M12 15v5" />
</svg>`

// TestInlineSVGKeepsShapeSizes checks that only the root element loses
// its size. Many icons are built from sized shapes -- Lucide's
// "workflow" is two rectangles and a connector -- and stripping those
// collapsed them, so the icon rendered as a fragment of itself.
func TestInlineSVGKeepsShapeSizes(t *testing.T) {
	const workflow = `<!-- @license lucide-static v1.28.0 - ISC -->
<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor">
  <rect width="8" height="8" x="3" y="3" rx="2" />
  <path d="M7 11v4a2 2 0 0 0 2 2h4" />
  <rect width="8" height="8" x="13" y="13" rx="2" />
</svg>`

	got := string(inlineSVG(workflow))
	if strings.Count(got, `width="8"`) != 2 || strings.Count(got, `height="8"`) != 2 {
		t.Errorf("inlineSVG dropped the sizes of the shapes inside the icon:\n%s", got)
	}
	// The root is still unsized, so the stylesheet decides how big the
	// icon is.
	root, _, _ := strings.Cut(got, ">")
	for _, unwanted := range []string{`width="24"`, `height="24"`} {
		if strings.Contains(root, unwanted) {
			t.Errorf("root element kept %s:\n%s", unwanted, root)
		}
	}
	if strings.Contains(got, "<!--") {
		t.Errorf("license comment was not removed:\n%s", got)
	}
	if !strings.Contains(got, `viewBox="0 0 24 24"`) {
		t.Errorf("viewBox was removed, so the icon has no coordinate system:\n%s", got)
	}
}

func TestPrepareIcons(t *testing.T) {
	dir := writeIconSet(t, map[string]string{
		"rocket":         testRocketSVG,
		"graduation-cap": `<svg viewBox="0 0 24 24"><path d="M1 1"/></svg>`,
	})

	cfg, err := prepareIcons(Config{Icons: dir})
	if err != nil {
		t.Fatalf("prepareIcons() error = %v", err)
	}
	if len(cfg.iconSet) != 2 {
		t.Fatalf("loaded %d icons, want 2", len(cfg.iconSet))
	}
	if got := cfg.navIcon("no-such-icon"); got != "" {
		t.Errorf("navIcon(unknown) = %q, want empty", got)
	}

	rocket := string(cfg.navIcon("rocket"))
	for _, unwanted := range []string{`width="24"`, `height="24"`, "\n"} {
		if strings.Contains(rocket, unwanted) {
			t.Errorf("inlined icon still contains %q:\n%s", unwanted, rocket)
		}
	}
	if !strings.Contains(rocket, "<!-- @license example - ISC -->") {
		t.Errorf("operator-supplied icon lost its attribution comment:\n%s", rocket)
	}
	for _, wanted := range []string{`viewBox="0 0 24 24"`, `stroke="currentColor"`, "<path"} {
		if !strings.Contains(rocket, wanted) {
			t.Errorf("inlined icon is missing %q:\n%s", wanted, rocket)
		}
	}
}

// TestPrepareIconsErrors checks that a misconfigured icon directory is
// reported rather than quietly rendering every page without icons.
func TestPrepareIconsErrors(t *testing.T) {
	tests := []struct {
		name string
		dir  func(t *testing.T) string
	}{
		{"missing directory", func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "nope")
		}},
		{"no svg files", func(t *testing.T) string {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			return dir
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := prepareIcons(Config{Icons: tt.dir(t)}); err == nil {
				t.Fatal("prepareIcons() succeeded, want an error")
			}
		})
	}
}

// isolateIconHome points the tail of the icon search path -- the user's
// home and configuration directories -- at an empty directory. Without
// it a developer who has an icon set installed there sees tests find it,
// and the same test passes on one machine and fails on another.
func isolateIconHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
}

// TestPrepareIconsUnset checks that Font Awesome is the deterministic
// default when no project or explicit icon directory is present.
func TestPrepareIconsUnset(t *testing.T) {
	isolateIconHome(t)
	cfg, err := prepareIcons(Config{})
	if err != nil {
		t.Fatalf("prepareIcons() error = %v", err)
	}
	if got := cfg.navIcon("rocket"); got == "" {
		t.Error("navIcon() did not use the built-in Font Awesome set")
	}
}

func TestPrepareBuiltinIconLibraries(t *testing.T) {
	for _, library := range []string{"fontawesome", "lucide", "tabler"} {
		t.Run(library, func(t *testing.T) {
			cfg, err := prepareBuiltinIcons(Config{}, library)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.navIcon("rocket") == "" {
				t.Errorf("%s has no rocket icon", library)
			}
			if cfg.iconAttribution == "" {
				t.Errorf("%s has no attribution", library)
			}
		})
	}
}

func TestPrepareIconsLibrarySelection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "docs.json"), []byte(`{"icons":{"library":"lucide"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := prepareIcons(Config{Source: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.iconAttribution; !strings.Contains(got, "Lucide") {
		t.Errorf("docs.json selected attribution %q, want Lucide", got)
	}
}

func TestFontAwesomeStyles(t *testing.T) {
	cfg, err := prepareBuiltinIcons(Config{}, "fontawesome")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.navIconType("clock", "regular") == "" {
		t.Error("regular clock did not resolve")
	}
	if cfg.navIconType("apple", "brands") == "" {
		t.Error("brands apple did not resolve")
	}
	if got := cfg.navIconType("rocket", "light"); got != "" {
		t.Errorf("unsupported explicit light style resolved to %q", got)
	}
	if got := cfg.navIconType("rocket", "regular"); got != "" {
		t.Errorf("missing regular rocket silently fell back to another style: %q", got)
	}
}

func TestNoIcons(t *testing.T) {
	cfg, err := prepareIcons(Config{NoIcons: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.navIcon("rocket"); got != "" {
		t.Errorf("navIcon() with NoIcons = %q, want empty", got)
	}
}

func TestMissingIconWarnsOnce(t *testing.T) {
	var log strings.Builder
	cfg, err := prepareBuiltinIcons(Config{}, "fontawesome")
	if err != nil {
		t.Fatal(err)
	}
	cfg.iconLogger = slog.New(slog.NewTextHandler(&log, nil))
	cfg.navIcon("not-a-real-icon")
	cfg.navIcon("not-a-real-icon")
	if got := strings.Count(log.String(), "Icon not found"); got != 1 {
		t.Errorf("missing-icon warnings = %d, want 1:\n%s", got, log.String())
	}
}

func TestIconSearchPathHasNoGlobalDirectories(t *testing.T) {
	root := t.TempDir()
	for _, dir := range iconSearchPath(root) {
		if !strings.HasPrefix(dir, root+string(filepath.Separator)) {
			t.Errorf("icon search path contains machine-global directory %q", dir)
		}
	}
}

func TestFontAwesomeCorpus(t *testing.T) {
	cfg, err := prepareBuiltinIcons(Config{}, "fontawesome")
	if err != nil {
		t.Fatal(err)
	}
	icons := []string{
		"apple", "arrows-turn-to-dots", "book-open", "box-archive", "boxes-stacked",
		"bullseye", "burst", "calendar-check", "chart-line", "clock", "code",
		"comments", "cube", "cubes", "diagram-next", "diagram-project", "flask",
		"folder-tree", "gauge-high", "graduation-cap", "hammer", "house", "life-ring",
		"list", "map", "microchip", "microscope", "network-wired", "play", "right-left",
		"rocket", "ruler-combined", "screwdriver-wrench", "shapes", "table", "table-cells",
		"triangle-exclamation", "wrench",
	}
	for _, icon := range icons {
		if cfg.navIcon(icon) == "" {
			t.Errorf("Font Awesome default does not resolve %q", icon)
		}
	}
}

// TestResolveIcon checks that a name is matched against the aliases for
// the same glyph, so documentation naming Font Awesome icons renders
// against a Lucide directory and the other way round.
func TestResolveIcon(t *testing.T) {
	lucide := map[string]template.HTML{
		"circle-help":   "<svg>help</svg>",
		"flask-conical": "<svg>flask</svg>",
		"rocket":        "<svg>rocket</svg>",
	}
	fontAwesome := map[string]template.HTML{
		"circle-question": "<svg>question</svg>",
		"vial":            "<svg>vial</svg>",
	}
	tests := []struct {
		name string
		set  map[string]template.HTML
		icon string
		want template.HTML
	}{
		{"exact name wins", lucide, "rocket", "<svg>rocket</svg>"},
		{"font awesome name against a lucide set", lucide, "circle-question", "<svg>help</svg>"},
		{"lucide name against a font awesome set", fontAwesome, "circle-help", "<svg>question</svg>"},
		{"second alias is tried", lucide, "vial", "<svg>flask</svg>"},
		{"no alias and no file", lucide, "no-such-icon", ""},
		{"alias exists but set has neither", fontAwesome, "gauge-high", ""},
		{"empty set", nil, "rocket", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveIcon(tt.set, tt.icon); got != tt.want {
				t.Errorf("resolveIcon(%q) = %q, want %q", tt.icon, got, tt.want)
			}
		})
	}
}

// TestResolveFontAwesomeIcons checks Font Awesome names whose Lucide
// counterparts use different names. These names are used by Mintlify
// documentation in the wild.
func TestResolveFontAwesomeIcons(t *testing.T) {
	tests := []struct {
		fontAwesome string
		lucide      string
	}{
		{"arrows-turn-to-dots", "workflow"},
		{"boxes-stacked", "boxes"},
		{"bullseye", "target"},
		{"burst", "badge"},
		{"cube", "box"},
		{"right-left", "arrow-right-left"},
		{"ruler-combined", "ruler"},
		{"table-cells", "table-2"},
	}
	for _, tt := range tests {
		t.Run(tt.fontAwesome, func(t *testing.T) {
			set := map[string]template.HTML{tt.lucide: "<svg/>"}
			if got := resolveIcon(set, tt.fontAwesome); got != "<svg/>" {
				t.Errorf("resolveIcon(%q) against Lucide %q = %q, want the glyph", tt.fontAwesome, tt.lucide, got)
			}
		})
	}
}

// TestIconAliasesAreNotSelfReferential checks that no alias points at
// the name it is listed under, which would be a lookup that can never
// add anything.
func TestIconAliasesAreNotSelfReferential(t *testing.T) {
	for name, aliases := range iconAliases {
		for _, alias := range aliases {
			if alias == name {
				t.Errorf("%q lists itself as an alias", name)
			}
		}
	}
}

// TestIconGroupsAreDisjoint checks that no name appears in two groups.
// A name in two would resolve to a different glyph depending on which
// group was consulted first.
func TestIconGroupsAreDisjoint(t *testing.T) {
	seen := make(map[string]int)
	for i, group := range iconGroups {
		if len(group) < 2 {
			t.Errorf("group %d has %d names; a group needs at least two to alias anything", i, len(group))
		}
		for _, name := range group {
			if prev, dup := seen[name]; dup {
				t.Errorf("%q appears in groups %d and %d", name, prev, i)
				continue
			}
			seen[name] = i
		}
	}
}

// TestIconAliasesAreSymmetric checks that every name in a group reaches
// every other one. Aliases used to be written by hand in one direction,
// so "diagram-project" found a Lucide "workflow" while a page naming
// "workflow" found nothing in a Font Awesome directory.
func TestIconAliasesAreSymmetric(t *testing.T) {
	for _, group := range iconGroups {
		for _, from := range group {
			for _, to := range group {
				if from == to {
					continue
				}
				set := map[string]template.HTML{to: template.HTML("<svg>" + to + "</svg>")}
				want := template.HTML("<svg>" + to + "</svg>")
				if got := resolveIcon(set, from); got != want {
					t.Errorf("resolveIcon(%q) against a set holding only %q = %q, want %q", from, to, got, want)
				}
			}
		}
	}
}

// TestResolveIconDiagramProject pins the case that prompted grouping:
// the same page renders against a Font Awesome, Lucide, or Tabler
// directory.
func TestResolveIconDiagramProject(t *testing.T) {
	sets := map[string]string{
		"font awesome": "diagram-project",
		"lucide":       "workflow",
		"tabler":       "sitemap",
	}
	for setName, file := range sets {
		for _, asked := range []string{"diagram-project", "workflow", "sitemap"} {
			t.Run(setName+"/"+asked, func(t *testing.T) {
				set := map[string]template.HTML{file: "<svg/>"}
				if got := resolveIcon(set, asked); got != "<svg/>" {
					t.Errorf("icon %q against the %s set = %q, want the glyph", asked, setName, got)
				}
			})
		}
	}
}

// TestFindIconsSearchPath checks that an icon set is found without
// -icons, nearest first, so a site that ships icons draws them without
// being told to.
func TestFindIconsSearchPath(t *testing.T) {
	const svg = `<svg viewBox="0 0 24 24"><path d="M1 1"/></svg>`

	isolateIconHome(t)

	t.Run("beside the documentation", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		icons := filepath.Join(root, "icons")
		if err := os.MkdirAll(icons, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(icons, "rocket.svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := prepareIcons(Config{Source: root})
		if err != nil {
			t.Fatalf("prepareIcons() error = %v", err)
		}
		if cfg.navIcon("rocket") == "" {
			t.Errorf("icons beside the documentation were not found; searched %v", iconSearchPath(root))
		}
	})

	t.Run("at the site root", func(t *testing.T) {
		siteDir := t.TempDir()
		docsDir := filepath.Join(siteDir, "docs")
		if err := os.MkdirAll(docsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# A\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(siteDir, docsJSONName), []byte(`{"name":"S"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		icons := filepath.Join(siteDir, "icons")
		if err := os.MkdirAll(icons, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(icons, "rocket.svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := prepareIcons(Config{Source: docsDir})
		if err != nil {
			t.Fatalf("prepareIcons() error = %v", err)
		}
		if cfg.navIcon("rocket") == "" {
			t.Errorf("icons at the site root were not found; searched %v", iconSearchPath(docsDir))
		}
	})

	// An "icons" directory holding no SVG files must not be taken as an
	// empty set, or it would mask a real one further along the path.
	t.Run("empty directory is passed over", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "icons"), 0o755); err != nil {
			t.Fatal(err)
		}
		dir, set := findIcons(root)
		if dir == filepath.Join(root, "icons") || len(set) != 0 {
			t.Errorf("findIcons() accepted an empty directory: %q, %d icons", dir, len(set))
		}
	})

	// No project icons falls back to the deterministic built-in set.
	t.Run("nothing found uses built-in icons", func(t *testing.T) {
		cfg, err := prepareIcons(Config{Source: t.TempDir()})
		if err != nil {
			t.Fatalf("prepareIcons() error = %v", err)
		}
		if cfg.navIcon("rocket") == "" {
			t.Error("built-in Font Awesome icon was not found")
		}
	})

	// An explicit -icons still wins, and still reports a bad path.
	t.Run("explicit flag overrides discovery", func(t *testing.T) {
		root := t.TempDir()
		beside := filepath.Join(root, "icons")
		if err := os.MkdirAll(beside, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(beside, "rocket.svg"), []byte("<svg>near</svg>"), 0o644); err != nil {
			t.Fatal(err)
		}
		named := writeIconSet(t, map[string]string{"rocket": "<svg>named</svg>"})
		cfg, err := prepareIcons(Config{Source: root, Icons: named})
		if err != nil {
			t.Fatalf("prepareIcons() error = %v", err)
		}
		if got := cfg.navIcon("rocket"); got != "<svg>named</svg>" {
			t.Errorf("navIcon() = %q, want the set named with -icons", got)
		}
		if _, err := prepareIcons(Config{Source: root, Icons: filepath.Join(root, "nope")}); err == nil {
			t.Error("a mistyped -icons succeeded, want an error")
		}
	})
}
