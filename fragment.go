package md2html

import (
	"html/template"
	"regexp"
	"strings"
)

// FragmentOptions configures fragment rendering.
type FragmentOptions struct {
	// AllowUnsafe permits raw HTML in rendered Markdown.
	AllowUnsafe bool
	// TOC enables table-of-contents generation for the fragment.
	TOC bool
	// HTMLExt is the extension used when rewriting local Markdown links.
	HTMLExt string
	// Format selects an opt-in structured Markdown presentation profile.
	// The empty string uses ordinary Markdown behavior. "okf" enables
	// Open Knowledge Format link handling.
	Format string
	// Frontmatter supplies page frontmatter used by fragment enhancements.
	Frontmatter map[string]any
}

// Fragment contains rendered markdown plus enhancement metadata for clients.
type Fragment struct {
	// HTML is the rendered Markdown fragment.
	HTML template.HTML
	// ChromaCSS is the syntax-highlighting stylesheet needed by HTML.
	ChromaCSS template.CSS
	// HasMath reports whether the source appears to contain MathJax syntax.
	HasMath bool
	// HasMermaid reports whether the source appears to contain Mermaid fences.
	HasMermaid bool
	// MermaidTheme is the light Mermaid theme.
	MermaidTheme string
	// MermaidDarkTheme is the dark Mermaid theme.
	MermaidDarkTheme string
	// MermaidAutoTheme reports whether client code should follow color scheme.
	MermaidAutoTheme bool
}

var (
	mathBlockPattern = regexp.MustCompile(`(?s)\$\$.*?\$\$|\\\[.*?\\\]`)
	// Inline $...$ follows Pandoc's rule to avoid currency false positives:
	// the opening $ needs a non-space character to its right, the closing $
	// a non-space character to its left, and the closing $ must not be
	// immediately followed by a digit ("$3 billion ... for $1000" is prose).
	mathInlinePattern = regexp.MustCompile(`(^|[^\\$])\$[^\s$](?:[^$]*[^\s$])?\$([^0-9]|$)|\\\((.+?)\\\)`)
	mermaidPattern    = regexp.MustCompile("(?m)^```mermaid\\s*$")
	codeRegionPattern = regexp.MustCompile(`(?s)<pre.*?</pre>|<code.*?</code>`)
)

// pageHasMath reports whether rendered HTML contains MathJax-style TeX
// delimiters outside <pre> and <code> regions, which MathJax skips.
func pageHasMath(htmlContent string) bool {
	return hasMath(codeRegionPattern.ReplaceAllString(htmlContent, ""))
}

// RenderFragment renders Markdown using the md2html pipeline and returns
// client enhancement metadata for MathJax and Mermaid. An invalid Format is
// treated as ordinary Markdown because fragment rendering has no error result.
func RenderFragment(markdown, filePath string, opts FragmentOptions) Fragment {
	if validateFormat(opts.Format) != nil {
		opts.Format = ""
	}
	cfg := Config{
		AllowUnsafe: opts.AllowUnsafe,
		TOC:         opts.TOC,
		HTMLExt:     opts.HTMLExt,
		Format:      opts.Format,
	}
	theme, darkTheme, autoTheme := resolveMermaidThemes(opts.Frontmatter)
	html, _ := markdownToHTMLWithContext(cfg, markdown, filePath)
	return Fragment{
		HTML:             template.HTML(html),
		ChromaCSS:        template.CSS(generateChromaCSS()),
		HasMath:          hasMath(markdown),
		HasMermaid:       mermaidPattern.MatchString(markdown),
		MermaidTheme:     theme,
		MermaidDarkTheme: darkTheme,
		MermaidAutoTheme: autoTheme,
	}
}

// FragmentStyles returns shared CSS for rendered markdown fragments.
func FragmentStyles() template.CSS {
	const mermaidCSS = `
.mermaid {
	max-width: 100%;
	overflow-x: auto;
	margin: 16px 0;
	border: 1px solid rgba(33, 37, 41, 0.08);
	border-radius: 12px;
	padding: 16px;
	background: rgba(248, 245, 238, 0.9);
}

.mermaid svg {
	max-width: 100%;
	height: auto;
	display: block;
	margin: 0 auto;
}
`
	return template.CSS(generateChromaCSS() + mermaidCSS)
}

func hasMath(markdown string) bool {
	if mathBlockPattern.MatchString(markdown) {
		return true
	}
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		if mathInlinePattern.MatchString(line) {
			return true
		}
	}
	return false
}
