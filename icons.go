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
// draws from, looking in the conventional places when no directory was
// named.
//
// A directory named with -icons that cannot be read is a configuration
// error: a mistyped path should say so rather than render every page
// without icons. A conventional directory that is not there is not an
// error, since not every site has one.
func prepareIcons(cfg Config) (Config, error) {
	if cfg.iconSet != nil {
		return cfg, nil
	}
	if strings.TrimSpace(cfg.Icons) == "" {
		dir, set := findIcons(cfg.Source)
		cfg.Icons, cfg.iconSet = dir, set
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

// iconDirName is the directory an icon set is kept in, both beside the
// documentation and in the user's configuration.
const iconDirName = "icons"

// iconSearchPath lists where an icon set is looked for when -icons was
// not given, nearest first: beside the documentation, then at the root
// of the published site, then in the user's configuration.
//
// Icons that ship with a site belong to it and should be found without
// being named. A set in the user's configuration is the fallback, so a
// preview of someone else's tree still draws icons.
func iconSearchPath(source string) []string {
	var dirs []string
	if root, err := sourceRoot(source); err == nil && root != "" {
		dirs = append(dirs, filepath.Join(root, iconDirName))
		if siteDir, found := findDocsJSON(root); found {
			dirs = append(dirs, filepath.Join(siteDir, iconDirName))
		}
	}
	if config, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(config, "md2html", iconDirName))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".md2html", iconDirName))
	}
	return dirs
}

// findIcons returns the first icon set on the search path. A directory
// that holds no SVG files is passed over rather than accepted as an
// empty set, so an unrelated "icons" directory does not mask the one
// further along.
func findIcons(source string) (string, map[string]template.HTML) {
	for _, dir := range iconSearchPath(source) {
		set, err := loadIcons(dir)
		if err != nil || len(set) == 0 {
			continue
		}
		return dir, set
	}
	return "", nil
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
