package md2html

import (
	"fmt"
	"html"
	"strings"

	"github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/util"
)

// codeLanguageNames maps a fence language to the name people write it as.
// Anything absent is shown as the author typed it, upper-cased, which is
// right for the long tail (rust, zig, toml) and wrong only for the few
// names that carry deliberate punctuation or case.
var codeLanguageNames = map[string]string{
	"bash":       "Bash",
	"c":          "C",
	"cpp":        "C++",
	"csharp":     "C#",
	"css":        "CSS",
	"diff":       "Diff",
	"docker":     "Dockerfile",
	"dockerfile": "Dockerfile",
	"go":         "Go",
	"graphql":    "GraphQL",
	"html":       "HTML",
	"java":       "Java",
	"javascript": "JavaScript",
	"js":         "JavaScript",
	"json":       "JSON",
	"jsonc":      "JSON",
	"kotlin":     "Kotlin",
	"markdown":   "Markdown",
	"md":         "Markdown",
	"objc":       "Objective-C",
	"php":        "PHP",
	"protobuf":   "Protobuf",
	"python":     "Python",
	"py":         "Python",
	"ruby":       "Ruby",
	"rb":         "Ruby",
	"rust":       "Rust",
	"scala":      "Scala",
	"sh":         "Shell",
	"shell":      "Shell",
	"sql":        "SQL",
	"swift":      "Swift",
	"toml":       "TOML",
	"ts":         "TypeScript",
	"tsx":        "TSX",
	"typescript": "TypeScript",
	"xml":        "XML",
	"yaml":       "YAML",
	"yml":        "YAML",
	"zsh":        "Zsh",
}

// codeLanguageLabel returns the display name for a fence language, or the
// empty string when the fence carried no language worth labelling.
func codeLanguageLabel(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" || lang == "text" || lang == "plain" || lang == "plaintext" {
		return ""
	}
	if name, ok := codeLanguageNames[strings.ToLower(lang)]; ok {
		return name
	}
	return strings.ToUpper(lang)
}

// codeBlockWrapper wraps every highlighted code block in a container
// carrying the fence language, so a reader can tell Go from shell without
// reading the code and the copy button has somewhere to sit. inner, when
// non-nil, renders inside the container and keeps its own wrapping.
func codeBlockWrapper(inner highlighting.WrapperRenderer) highlighting.WrapperRenderer {
	return func(w util.BufWriter, ctx highlighting.CodeBlockContext, entering bool) {
		lang, ok := ctx.Language()
		label := ""
		if ok {
			label = codeLanguageLabel(string(lang))
		}

		if entering {
			w.WriteString(`<div class="md-code"`)
			if label != "" {
				fmt.Fprintf(w, ` data-language="%s"`, html.EscapeString(label))
			}
			w.WriteString(">\n")
			if label != "" {
				fmt.Fprintf(w, `<span class="md-code-lang" aria-hidden="true">%s</span>`+"\n", html.EscapeString(label))
			}
		}

		if inner != nil {
			inner(w, ctx, entering)
		} else if !ctx.Highlighted() {
			// The wrapper replaces the markup the highlighter would have
			// written itself, so the unhighlighted fallback has to be
			// reproduced here.
			if entering {
				w.WriteString(`<pre><code`)
				if ok {
					fmt.Fprintf(w, ` class="language-%s"`, html.EscapeString(string(lang)))
				}
				w.WriteByte('>')
			} else {
				w.WriteString("</code></pre>\n")
			}
		}

		if !entering {
			w.WriteString("</div>\n")
		}
	}
}
