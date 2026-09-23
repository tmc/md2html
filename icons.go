package md2html

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/tmc/md2html/internal/iconsets"
)

// prepareIcons resolves [Config.Icons] into the icon set the navigation
// draws from, looking in the conventional places when no directory was
// named.
//
// A directory named with -icons that cannot be read is a configuration
// error: a mistyped path should say so rather than render every page
// without icons. A conventional directory that is not there is not an
// error, since not every site has one.
func (s *preparedSite) prepareIcons() error {
	if s.iconSet != nil {
		return nil
	}
	if s.iconMissing == nil {
		s.iconMissing = new(sync.Map)
	}
	if s.config.NoIcons {
		s.iconSet = make(map[string]template.HTML)
		s.iconDisabled = true
		return nil
	}
	if strings.TrimSpace(s.config.Icons) == "" {
		dir, set := findIcons(s.base, s.config.Source)
		if len(set) != 0 {
			s.config.Icons, s.iconSet = dir, set
			return nil
		}
		return s.prepareBuiltinIcons(iconLibraryForSource(s.base, s.config.Source))
	}
	dir, err := filepath.Abs(s.config.Icons)
	if err != nil {
		return fmt.Errorf("resolve icons directory: %w", err)
	}
	set, err := loadIcons(dir)
	if err != nil {
		return fmt.Errorf("load icons: %w", err)
	}
	s.config.Icons = dir
	s.iconSet = set
	return nil
}

// prepareIcons prepares a site with only icons loaded from cfg.
func prepareIcons(cfg Config) (*preparedSite, error) {
	s, err := newPreparedSite(cfg, nil)
	if err != nil {
		return nil, err
	}
	if err := s.prepareIcons(); err != nil {
		return nil, err
	}
	return s, nil
}

// prepareBuiltinIcons prepares a site with only built-in icons loaded.
func prepareBuiltinIcons(cfg Config, name string) (*preparedSite, error) {
	s, err := newPreparedSite(cfg, nil)
	if err != nil {
		return nil, err
	}
	if err := s.prepareBuiltinIcons(name); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *preparedSite) prepareBuiltinIcons(name string) error {
	lib, err := builtinIcons(name)
	if err != nil {
		return err
	}
	s.iconAttribution = lib.attribution
	s.iconSet = lib.set
	s.iconStyles = lib.styles
	return nil
}

// inlinedLibrary is a built-in icon library prepared for inlining. Its
// maps are shared by every site that uses the library and must not be
// modified.
type inlinedLibrary struct {
	attribution string
	set         map[string]template.HTML
	styles      map[string]map[string]template.HTML // Font Awesome only
}

var builtinIconCache sync.Map // library name -> func() (*inlinedLibrary, error)

// builtinIcons returns the named built-in library, inlining it on first
// use. Every rendered fragment prepares a site, so redoing this work per
// site would dominate rendering time.
func builtinIcons(name string) (*inlinedLibrary, error) {
	f, _ := builtinIconCache.LoadOrStore(name, sync.OnceValues(func() (*inlinedLibrary, error) {
		return inlineLibrary(name)
	}))
	return f.(func() (*inlinedLibrary, error))()
}

func inlineLibrary(name string) (*inlinedLibrary, error) {
	lib, err := iconsets.Load(name)
	if err != nil {
		return nil, fmt.Errorf("load built-in icons: %w", err)
	}
	out := &inlinedLibrary{
		attribution: lib.Attribution,
		set:         make(map[string]template.HTML),
	}
	if name != "fontawesome" {
		for name, svg := range lib.Icons {
			out.set[name] = inlineSVG(svg)
		}
		return out, nil
	}
	styles := []string{"solid", "regular", "brands"}
	out.styles = make(map[string]map[string]template.HTML)
	for _, style := range styles {
		out.styles[style] = make(map[string]template.HTML)
	}
	for key, svg := range lib.Icons {
		style, name, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		out.styles[style][name] = inlineSVG(svg)
	}
	for _, style := range styles {
		for name, markup := range out.styles[style] {
			if _, exists := out.set[name]; !exists {
				out.set[name] = markup
			}
		}
	}
	return out, nil
}

// iconDirName is the directory an icon set is kept in beside documentation.
const iconDirName = "icons"

// iconSearchPath lists where an icon set is looked for when -icons was
// not given, nearest first: beside the documentation, then at the root
// of the published site.
//
// Icons that ship with a site belong to it and should be found without
// being named. Machine-global directories are deliberately omitted so
// the same source tree renders the same way on every machine.
func iconSearchPath(base, source string) []string {
	var dirs []string
	if root, err := sourceRoot(base, source); err == nil && root != "" {
		dirs = append(dirs, filepath.Join(root, iconDirName))
		if siteDir, found := findDocsJSON(root); found {
			dirs = append(dirs, filepath.Join(siteDir, iconDirName))
		}
	}
	return dirs
}

// findIcons returns the first icon set on the search path. A directory
// that holds no SVG files is passed over rather than accepted as an
// empty set, so an unrelated "icons" directory does not mask the one
// further along.
func findIcons(base, source string) (string, map[string]template.HTML) {
	for _, dir := range iconSearchPath(base, source) {
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
		set[name] = inlineCustomSVG(string(data))
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
// so this is presentation rather than sanitization.
func inlineSVG(svg string) template.HTML {
	return inlineSVGWithComments(svg, false)
}

// inlineCustomSVG keeps comments because an operator-supplied SVG may
// carry attribution required by its license.
func inlineCustomSVG(svg string) template.HTML {
	return inlineSVGWithComments(svg, true)
}

func inlineSVGWithComments(svg string, keepComments bool) template.HTML {
	if !keepComments {
		svg = svgComment.ReplaceAllString(svg, "")
	}
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
func (s *preparedSite) navIcon(name string) template.HTML {
	return s.navIconType(name, "")
}

func (s *preparedSite) navIconType(name, style string) template.HTML {
	if s == nil || s.iconDisabled {
		return ""
	}
	if style != "" && s.iconStyles != nil {
		set, ok := s.iconStyles[style]
		if !ok {
			s.warnMissingIcon(name, style)
			return ""
		}
		if svg := resolveIcon(set, name); svg != "" {
			return svg
		}
		s.warnMissingIcon(name, style)
		return ""
	}
	if svg := resolveIcon(s.iconSet, name); svg != "" {
		return svg
	}
	s.warnMissingIcon(name, style)
	return ""
}

func (s *preparedSite) hasIcon(name, style string) bool {
	if s == nil || s.iconDisabled {
		return false
	}
	if style != "" && s.iconStyles != nil {
		return resolveIcon(s.iconStyles[style], name) != ""
	}
	return resolveIcon(s.iconSet, name) != ""
}

func (s *preparedSite) warnMissingIcon(name, style string) {
	if s == nil || s.iconLogger == nil || s.iconMissing == nil {
		return
	}
	key := style + "/" + name
	if _, loaded := s.iconMissing.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	s.iconLogger.Warn("Icon not found", "name", name, "style", style)
}
