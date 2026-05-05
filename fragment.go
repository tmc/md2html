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
	// Frontmatter supplies page frontmatter used by fragment enhancements.
	Frontmatter map[string]interface{}
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
	mathBlockPattern  = regexp.MustCompile(`(?s)\$\$.*?\$\$|\\\[.*?\\\]`)
	mathInlinePattern = regexp.MustCompile(`(^|[^\\])\$(.+?)\$|\\\((.+?)\\\)`)
	mermaidPattern    = regexp.MustCompile("(?m)^```mermaid\\s*$")
)

// RenderFragment renders markdown using the md2html pipeline and returns
// client enhancement metadata for MathJax and Mermaid.
func RenderFragment(markdown, filePath string, opts FragmentOptions) Fragment {
	cfg := Config{
		AllowUnsafe: opts.AllowUnsafe,
		TOC:         opts.TOC,
		HTMLExt:     opts.HTMLExt,
	}
	theme, darkTheme, autoTheme := resolveMermaidThemes(opts.Frontmatter)
	return Fragment{
		HTML:             template.HTML(markdownToHTMLWithContext(cfg, markdown, filePath)),
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
