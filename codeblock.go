package md2html

import (
	"bytes"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
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

// writeCodeTitle writes the caption of a code block whose fence names
// one, as in
//
//	```go {title="main.go" href="https://example.com/main.go"}
//
// The href, which is optional, makes the caption a link. Only a web
// address or a relative one is accepted, since the attribute is as
// much the author's as any link in the page.
func writeCodeTitle(w util.BufWriter, ctx highlighting.CodeBlockContext) {
	attrs := ctx.Attributes()
	if attrs == nil {
		return
	}
	attr := func(name string) string {
		v, _ := attrs.Get([]byte(name))
		b, _ := v.([]byte)
		return string(b)
	}
	title, href := attr("title"), attr("href")
	if title == "" {
		return
	}
	if u, err := url.Parse(href); err != nil || (u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
		href = ""
	}
	w.WriteString(`<div class="md-code-title">`)
	if href != "" {
		fmt.Fprintf(w, `<a href="%s">%s</a>`, html.EscapeString(href), html.EscapeString(title))
	} else {
		w.WriteString(html.EscapeString(title))
	}
	w.WriteString("</div>\n")
}

// setFenceAttributes sets on n the attributes its fence names in
// braces, as in
//
//	```go {title="main.go" href="main.go"}
//
// The highlighter reads them from the fence itself only when n has no
// attributes of its own, so setting them on n lets a caption's link be
// rewritten as any other link in the page is, and keeps them when an
// extension has set attributes of its own. Those take precedence.
func setFenceAttributes(n *ast.FencedCodeBlock, source []byte) {
	if n.Info == nil {
		return
	}
	info := n.Info.Segment.Value(source)
	i := bytes.IndexByte(info, '{')
	if i <= 0 {
		return
	}
	attrs, ok := parser.ParseAttributes(text.NewReader(info[i:]))
	if !ok {
		return
	}
	for _, a := range attrs {
		if _, ok := n.Attribute(a.Name); !ok {
			n.SetAttribute(a.Name, a.Value)
		}
	}
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
			writeCodeTitle(w, ctx)
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
