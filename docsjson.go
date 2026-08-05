package md2html

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// docsJSONName is the Mintlify site configuration file. It sits at the
// root of the published site, which is often a parent of the Markdown
// directory being served.
const docsJSONName = "docs.json"

// defaultTitle is the -title default. A site name from docs.json replaces
// it, but never replaces a title the caller chose.
const defaultTitle = "Markdown Preview"

// siteTitle picks the site title: what the caller asked for, or the name
// the navigation source carries.
func siteTitle(configured, siteName string) string {
	if configured != "" && configured != defaultTitle {
		return configured
	}
	if siteName != "" {
		return siteName
	}
	return configured
}

// docsJSON is the part of a Mintlify docs.json that md2html reads.
type docsJSON struct {
	Name       string             `json:"name"`
	Colors     docsJSONColors     `json:"colors"`
	Navigation docsJSONNavigation `json:"navigation"`
	Navbar     docsJSONNavbar     `json:"navbar"`
}

// docsJSONNavbar is the bar above the page. Mintlify puts a single
// "primary" call to action in it, which for a source project is a link
// to the repository.
type docsJSONNavbar struct {
	Primary docsJSONNavbarLink `json:"primary"`
}

type docsJSONNavbarLink struct {
	Type string `json:"type"`
	Href string `json:"href"`
}

// docsJSONColors is the site palette. Mintlify names "primary" for the
// brand color and "light" for the lighter variant it uses on dark
// backgrounds.
type docsJSONColors struct {
	Primary string `json:"primary"`
	Light   string `json:"light"`
	Dark    string `json:"dark"`
}

type docsJSONNavigation struct {
	Groups []docsJSONGroup `json:"groups"`
	Pages  []docsJSONPage  `json:"pages"`
}

type docsJSONGroup struct {
	Group string         `json:"group"`
	Pages []docsJSONPage `json:"pages"`
}

// docsJSONPage is an entry in a "pages" list. Mintlify allows either a
// page path or a nested group, so the entry is decoded from whichever
// shape the JSON holds.
type docsJSONPage struct {
	Path  string
	Group *docsJSONGroup
}

func (p *docsJSONPage) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		p.Path = s
		return nil
	}
	var g docsJSONGroup
	if err := json.Unmarshal(data, &g); err != nil {
		return fmt.Errorf("page entry is neither a path nor a group: %w", err)
	}
	p.Group = &g
	return nil
}

// findDocsJSON looks for docs.json in dir and its parents, returning the
// directory holding it. Page paths in docs.json are relative to that
// directory, which is the root of the published site.
func findDocsJSON(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, docsJSONName)); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// siteInfo is the presentation carried by a navigation source: what the
// site is called and the color it is branded with.
type siteInfo struct {
	Name string
	// Repo is the "owner/name" the documented source lives at, and
	// RepoURL the address to link it to. Both are empty unless the
	// navigation source names a repository.
	Repo    string
	RepoURL string
	// Accent and AccentDark are CSS colors for light and dark rendering.
	// They are empty unless the source names a valid one.
	Accent     string
	AccentDark string
}

// hexColor matches the CSS hex colors md2html is willing to interpolate
// into a stylesheet. Anything else is ignored rather than escaped, since
// a rejected color simply leaves the built-in accent in place.
var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

func cssColor(v string) string {
	v = strings.TrimSpace(v)
	if hexColor.MatchString(v) {
		return v
	}
	return ""
}

// loadDocsJSON reads the docs.json covering sourceDir and returns the
// navigation it describes along with the site name and colors. It reports
// ok=false when there is no docs.json, when it does not parse, or when
// none of the pages it names live under sourceDir — a docs.json found
// several levels up may describe an unrelated tree.
func loadDocsJSON(sourceDir, htmlExt string) (nav *Navigation, site siteInfo, ok bool) {
	siteDir, found := findDocsJSON(sourceDir)
	if !found {
		return nil, siteInfo{}, false
	}
	data, err := os.ReadFile(filepath.Join(siteDir, docsJSONName))
	if err != nil {
		return nil, siteInfo{}, false
	}
	var doc docsJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, siteInfo{}, false
	}

	b := docsJSONBuilder{siteDir: siteDir, sourceDir: sourceDir, htmlExt: htmlExt}
	items := b.pages(doc.Navigation.Pages, 0)
	for _, g := range doc.Navigation.Groups {
		if item := b.group(g, 0); item != nil {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, siteInfo{}, false
	}

	site = siteInfo{Name: doc.Name, Accent: cssColor(doc.Colors.Primary)}
	site.Repo, site.RepoURL = repoLink(doc.Navbar.Primary)
	// Mintlify's "light" is the variant meant for dark backgrounds.
	site.AccentDark = cssColor(doc.Colors.Light)
	if site.AccentDark == "" {
		site.AccentDark = site.Accent
	}

	nav = &Navigation{Items: items}
	nav.buildIndexes()
	return nav, site, true
}

type docsJSONBuilder struct {
	siteDir   string
	sourceDir string
	htmlExt   string
}

func (b docsJSONBuilder) group(g docsJSONGroup, level int) *NavItem {
	children := b.pages(g.Pages, level+1)
	if len(children) == 0 {
		return nil
	}
	return &NavItem{Title: g.Group, IsGroup: true, Level: level, Children: children}
}

func (b docsJSONBuilder) pages(pages []docsJSONPage, level int) []*NavItem {
	var items []*NavItem
	for _, p := range pages {
		if p.Group != nil {
			if item := b.group(*p.Group, level); item != nil {
				items = append(items, item)
			}
			continue
		}
		if item := b.page(p.Path, level); item != nil {
			items = append(items, item)
		}
	}
	return items
}

// page resolves one docs.json page path, which is extensionless and
// relative to the site root, against the directory being served. Pages
// outside that directory are skipped: they belong to the site but not to
// this preview.
func (b docsJSONBuilder) page(pagePath string, level int) *NavItem {
	if pagePath == "" {
		return nil
	}
	rel, err := filepath.Rel(b.sourceDir, filepath.Join(b.siteDir, filepath.FromSlash(pagePath)))
	if err != nil {
		return nil
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return nil
	}

	for _, ext := range []string{".md", ".markdown", ".mdx"} {
		source := rel + ext
		full := filepath.Join(b.sourceDir, filepath.FromSlash(source))
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		docData, err := parseFrontmatter(string(data))
		if err != nil {
			docData = DocumentData{Content: string(data), Frontmatter: map[string]any{}}
		}
		stem := strings.TrimSuffix(path.Base(source), path.Ext(source))
		return &NavItem{
			Title: autoNavTitle(stem, docData),
			Icon:  navIcon(docData),
			Path:  source,
			URL:   pathToURL(source, b.htmlExt),
			Level: level,
		}
	}
	return nil
}


// githubRepo matches the repository page of a GitHub URL, capturing the
// owner and the repository name.
var githubRepo = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/?#]+)`)

// repoLink reports the repository a navbar link points at, as the
// "owner/name" to show and the URL to link to. A link that is not a
// GitHub repository yields nothing rather than a guess: the label is
// meant to read as a repository, and only GitHub URLs are recognised
// well enough to say so.
func repoLink(link docsJSONNavbarLink) (repo, url string) {
	if link.Type != "github" {
		return "", ""
	}
	m := githubRepo.FindStringSubmatch(strings.TrimSpace(link.Href))
	if m == nil {
		return "", ""
	}
	name := strings.TrimSuffix(m[2], ".git")
	return m[1] + "/" + name, "https://github.com/" + m[1] + "/" + name
}
