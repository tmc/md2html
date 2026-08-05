package md2html

import (
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
	for _, unwanted := range []string{`width="24"`, `height="24"`, "<!--", "\n"} {
		if strings.Contains(rocket, unwanted) {
			t.Errorf("inlined icon still contains %q:\n%s", unwanted, rocket)
		}
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

// TestPrepareIconsUnset checks that no icon directory is not an error:
// icons are optional, and pages naming one simply render without it.
func TestPrepareIconsUnset(t *testing.T) {
	cfg, err := prepareIcons(Config{})
	if err != nil {
		t.Fatalf("prepareIcons() error = %v", err)
	}
	if got := cfg.navIcon("rocket"); got != "" {
		t.Errorf("navIcon() = %q with no icon set, want empty", got)
	}
}
