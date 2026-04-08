package md2html

import (
	"html/template"
	"regexp"
	"strings"
)

// FragmentOptions configures fragment rendering.
type FragmentOptions struct {
	AllowUnsafe bool
	TOC         bool
	HTMLExt     string
	Frontmatter map[string]interface{}
}

// Fragment contains rendered markdown plus enhancement metadata for clients.
type Fragment struct {
	HTML             template.HTML
	ChromaCSS        template.CSS
	HasMath          bool
	HasMermaid       bool
	MermaidTheme     string
	MermaidDarkTheme string
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
