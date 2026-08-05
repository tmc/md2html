package md2html

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// prepareIcons resolves [Config.Icons] into the icon set the navigation
// draws from. A directory that cannot be read is a configuration error:
// silently rendering every page without icons would look like the pages
// forgot to ask for them.
func prepareIcons(cfg Config) (Config, error) {
	if cfg.iconSet != nil || strings.TrimSpace(cfg.Icons) == "" {
		return cfg, nil
	}
	dir, err := filepath.Abs(cfg.Icons)
	if err != nil {
		return cfg, fmt.Errorf("resolve icons directory: %w", err)
	}
	set, err := loadIcons(dir)
	if err != nil {
		return cfg, fmt.Errorf("load icons: %w", err)
	}
	cfg.Icons = dir
	cfg.iconSet = set
	return cfg, nil
}

// loadIcons reads every .svg in dir, keyed by file name without the
// extension. That is the name pages use in frontmatter, so an icon set
// is any directory of SVG files named the way the docs name them.
func loadIcons(dir string) (map[string]template.HTML, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.svg"))
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no .svg files in %s", dir)
	}
	set := make(map[string]template.HTML, len(entries))
	for _, entry := range entries {
		data, err := os.ReadFile(entry)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(entry), ".svg")
		set[name] = inlineSVG(string(data))
	}
	return set, nil
}

var (
	svgComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	svgRootTag = regexp.MustCompile(`(?s)<svg\b[^>]*>`)
	svgSize    = regexp.MustCompile(`\s(?:width|height)="[^"]*"`)
	svgSpace   = regexp.MustCompile(`\s+`)
)

// inlineSVG prepares an icon file for inlining: the fixed pixel size
// comes off so CSS controls it, comments come off so a license header is
// not repeated into every page, and the whitespace icon sets use for
// readability is collapsed.
//
// Icons are operator-supplied configuration, like the template directory,
// so this is presentation rather than sanitisation.
func inlineSVG(svg string) template.HTML {
	svg = svgComment.ReplaceAllString(svg, "")
	// Only the root element is resized. Shapes inside an icon carry
	// width and height of their own -- a Lucide "workflow" is two
	// rectangles and a connector -- and stripping those collapses them
	// to nothing, leaving a fragment of the glyph.
	svg = replaceFirst(svg, svgRootTag, func(tag string) string {
		return svgSize.ReplaceAllString(tag, "")
	})
	svg = svgSpace.ReplaceAllString(svg, " ")
	return template.HTML(strings.TrimSpace(svg))
}

// replaceFirst rewrites the first match of re in s using f.
func replaceFirst(s string, re *regexp.Regexp, f func(string) string) string {
	loc := re.FindStringIndex(s)
	if loc == nil {
		return s
	}
	return s[:loc[0]] + f(s[loc[0]:loc[1]]) + s[loc[1]:]
}

// navIcon returns the markup for an icon name, or the empty string when
// no icon set is configured or the set does not have that name. A name
// with no icon leaves the entry without one, which is how pages render
// when no set is configured at all.
func (cfg Config) navIcon(name string) template.HTML {
	return resolveIcon(cfg.iconSet, name)
}
