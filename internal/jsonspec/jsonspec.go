package jsonspec

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Config controls discriminator detection and badge rendering.
type Config struct {
	// DiscriminatorPrefixes lists type-value prefixes that mark a JSON
	// block for enrichment. For ASCF this is []string{"ascf/"}. When
	// empty the extension is a no-op.
	DiscriminatorPrefixes []string

	// BadgeURLTemplate is a printf-style URL template. A single %s is
	// substituted with the discriminator suffix (for example, a prefix
	// of "ascf/" and a type of "ascf/hypothesis" yields "hypothesis").
	// When empty no badge anchor is emitted; the wrapper div is still
	// rendered so client-side code can find it.
	BadgeURLTemplate string

	// BadgeLabelTemplate is a printf-style label template rendered as
	// the badge text. A single %s is substituted with the discriminator
	// suffix. Defaults to "%s" when empty.
	BadgeLabelTemplate string
}

// Attribute name set on the FencedCodeBlock by the AST transformer and
// read back by the WrapperRenderer.
var attrSchemaType = []byte("data-schema-type")

// discriminatorPattern matches "type"<ws>:<ws>"<value>".
// The value capture stops at the closing quote. We avoid full JSON
// parsing to stay tolerant of truncated or elided examples.
var discriminatorPattern = regexp.MustCompile(`"type"\s*:\s*"([^"]+)"`)

// Extension returns a goldmark.Extender that installs the AST
// transformer responsible for detecting discriminators and tagging the
// node. The badge + wrapper markup is emitted by WrapperRenderer, which
// must be installed on the highlighting extension separately.
func Extension(cfg Config) goldmark.Extender {
	return &extender{cfg: cfg}
}

type extender struct {
	cfg Config
}

func (e *extender) Extend(md goldmark.Markdown) {
	if len(e.cfg.DiscriminatorPrefixes) == 0 {
		return
	}
	md.Parser().AddOptions(parser.WithASTTransformers(
		util.Prioritized(&transformer{cfg: e.cfg}, 900),
	))
}

type transformer struct {
	cfg Config
}

func (t *transformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		lang := fcb.Language(source)
		if !isJSONLang(lang) {
			return ast.WalkContinue, nil
		}
		disc := t.detect(source, fcb)
		if disc == "" {
			return ast.WalkContinue, nil
		}
		fcb.SetAttribute(attrSchemaType, []byte(disc))
		return ast.WalkContinue, nil
	})
}

func (t *transformer) detect(source []byte, n *ast.FencedCodeBlock) string {
	var buf bytes.Buffer
	for i := range n.Lines().Len() {
		seg := n.Lines().At(i)
		buf.Write(seg.Value(source))
	}
	m := discriminatorPattern.FindSubmatch(buf.Bytes())
	if m == nil {
		return ""
	}
	val := string(m[1])
	for _, p := range t.cfg.DiscriminatorPrefixes {
		if rest, ok := strings.CutPrefix(val, p); ok {
			return trimSep(rest)
		}
	}
	return ""
}

// trimSep removes one leading namespace-separator character after the
// discriminator prefix has been cut. This makes prefix configuration
// forgiving: "ascf" and "ascf/" both produce the suffix "challenge"
// from the value "ascf/challenge".
func trimSep(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '/', ':', '.', '-', '_':
		return s[1:]
	}
	return s
}

// isJSONLang returns true for fences that declare a JSON language.
// "json", "jsonc", and "json5" all count; anything else is skipped so
// other languages don't get accidentally enriched by a substring match.
func isJSONLang(lang []byte) bool {
	switch strings.ToLower(string(lang)) {
	case "json", "jsonc", "json5":
		return true
	}
	return false
}

// WrapperRenderer returns a highlighting.WrapperRenderer that emits a
// wrapper <div> + optional badge around the chroma-highlighted code
// block whenever the transformer tagged it with data-schema-type.
func WrapperRenderer(cfg Config) highlighting.WrapperRenderer {
	return func(w util.BufWriter, ctx highlighting.CodeBlockContext, entering bool) {
		attrs := ctx.Attributes()
		var schemaType string
		if attrs != nil {
			if v, ok := attrs.Get(attrSchemaType); ok {
				schemaType = byteString(v)
			}
		}
		if entering {
			if schemaType != "" {
				fmt.Fprintf(w, `<div class="md-jsonspec" data-schema-type="%s">`, html.EscapeString(schemaType))
				w.WriteByte('\n')
				writeBadge(w, cfg, schemaType)
			}
			// Emit the default opening wrapper chroma would have written
			// itself when WrapperRenderer is absent. chroma's formatter
			// still emits its own inner <pre class="chroma"> around the
			// tokens; this outer <pre> is only used when chroma's lexer
			// is unavailable (see highlighting.renderFencedCodeBlock
			// fallback path).
			lang, ok := ctx.Language()
			if !ctx.Highlighted() {
				w.WriteString(`<pre><code`)
				if ok {
					fmt.Fprintf(w, ` class="language-%s"`, html.EscapeString(string(lang)))
				}
				w.WriteByte('>')
			}
			return
		}
		if !ctx.Highlighted() {
			w.WriteString("</code></pre>\n")
		}
		if schemaType != "" {
			w.WriteString("</div>\n")
		}
	}
}

func writeBadge(w util.BufWriter, cfg Config, schemaType string) {
	label := schemaType
	if cfg.BadgeLabelTemplate != "" {
		label = fmt.Sprintf(cfg.BadgeLabelTemplate, schemaType)
	}
	if cfg.BadgeURLTemplate == "" {
		fmt.Fprintf(w, `<span class="md-jsonspec-badge">%s</span>`, html.EscapeString(label))
		w.WriteByte('\n')
		return
	}
	href := fmt.Sprintf(cfg.BadgeURLTemplate, schemaType)
	fmt.Fprintf(w, `<a class="md-jsonspec-badge" href="%s">%s</a>`,
		html.EscapeString(href),
		html.EscapeString(label),
	)
	w.WriteByte('\n')
}

// byteString converts an attribute value to a string. Goldmark stores
// attribute values as either []byte or string depending on where they
// came from; we accept both.
func byteString(v any) string {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case string:
		return x
	}
	return ""
}
