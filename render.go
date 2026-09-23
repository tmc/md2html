package md2html

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	admonitions "github.com/stefanfritsch/goldmark-admonitions"
	"github.com/tmc/md2html/internal/anchor"
	"github.com/tmc/md2html/internal/markdown/components"
	"github.com/tmc/md2html/internal/markdown/jsonspec"
	"github.com/tmc/md2html/internal/markdown/media"
	"github.com/tmc/md2html/internal/markdown/tabs"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/toc"
	"gopkg.in/yaml.v3"
)

//go:embed all:templates
var templates embed.FS

var htmlLinkAttrPattern = regexp.MustCompile(`(?i)\b(href|src)\s*=\s*(?:"([^"<>]+)"|'([^'<>]+)')`)

var alertKindAttr = []byte("data-md2html-alert")

func generateChromaCSS() string {
	lightStyle := styles.Get("github")
	if lightStyle == nil {
		lightStyle = styles.Fallback
	}

	darkStyle := styles.Get("github-dark")
	if darkStyle == nil {
		if darkStyle = styles.Get("dracula"); darkStyle == nil {
			darkStyle = lightStyle
		}
	}

	formatter := chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.WithLineNumbers(false),
	)

	var light, dark bytes.Buffer
	if err := formatter.WriteCSS(&light, lightStyle); err != nil {
		return ""
	}
	if err := formatter.WriteCSS(&dark, darkStyle); err != nil {
		return ""
	}

	lightCSS := withoutCanvas(light.String())
	darkCSS := withoutCanvas(dark.String())

	// The syntax colors have to follow the same conditions as the design
	// tokens, or a reader who forces one theme gets the other theme's
	// code: dark-background keywords on the light page surface, which is
	// unreadable. Each stylesheet is scoped the way the tokens are:
	// what the system prefers, unless the reader forced the other.
	//
	// Both are scoped, not just one. The two styles do not name the same
	// token classes, so whichever was left unscoped would show through
	// wherever the other is silent: github colors NameOther near-black
	// and github-dark says nothing about it, which left every plain
	// identifier in a dark block black on black.
	var buf bytes.Buffer
	buf.WriteString("@media (prefers-color-scheme: dark) {\n")
	buf.WriteString(scopeCSS(darkCSS, `:root:not([data-theme="light"])`))
	buf.WriteString("}\n\n")
	buf.WriteString(scopeCSS(darkCSS, `:root[data-theme="dark"]`))

	buf.WriteString("\n@media (prefers-color-scheme: light), (prefers-color-scheme: no-preference) {\n")
	buf.WriteString(scopeCSS(lightCSS, `:root:not([data-theme="dark"])`))
	buf.WriteString("}\n\n")
	buf.WriteString(scopeCSS(lightCSS, `:root[data-theme="light"]`))

	return buf.String()
}

// withoutCanvas drops the rules that paint chroma's own page color
// behind a block. The page already gives code a surface from its design
// tokens; chroma's is a shade off from it, and once the stylesheet is
// scoped to a theme it outweighs the page's own rule.
func withoutCanvas(css string) string {
	var buf strings.Builder
	for line := range strings.SplitSeq(css, "\n") {
		if strings.HasPrefix(line, "/* Background */") || strings.HasPrefix(line, "/* PreWrapper */") {
			continue
		}
		if line == "" {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return buf.String()
}

// scopeCSS narrows every rule in css to descendants of scope. chroma
// writes one rule per line, an optional "/* Token */" comment followed
// by a selector list, so each line's selectors are prefixed in place.
// A line that holds no rule is copied through.
func scopeCSS(css, scope string) string {
	var buf strings.Builder
	for line := range strings.SplitSeq(css, "\n") {
		open := strings.IndexByte(line, '{')
		if open < 0 {
			buf.WriteString(line)
			buf.WriteByte('\n')
			continue
		}
		head, body := line[:open], line[open:]
		if end := strings.LastIndex(head, "*/"); end >= 0 {
			buf.WriteString(head[:end+2])
			buf.WriteByte(' ')
			head = head[end+2:]
		}
		for i, sel := range strings.Split(head, ",") {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(scope)
			buf.WriteByte(' ')
			buf.WriteString(strings.TrimSpace(sel))
		}
		buf.WriteByte(' ')
		buf.WriteString(body)
		buf.WriteByte('\n')
	}
	return buf.String()
}

func preprocessHTMLBlocks(markdown string) string {
	blockTags := []string{"<div", "<dl", "<table", "<section"}
	lines := strings.Split(markdown, "\n")
	var result []string
	depth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, tag := range blockTags {
			if strings.Contains(trimmed, tag) {
				depth++
			}
			if strings.Contains(trimmed, "</"+tag[1:]+">") {
				depth--
			}
		}
		if depth > 0 {
			result = append(result, trimmed)
		} else {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// typographer returns the smart-punctuation extension, configured to
// match what hosted documentation platforms produce rather than
// goldmark's defaults, so the same source reads the same either way.
//
// The two disagree about dashes. goldmark reads "--" as an en dash and
// "---" as an em dash; the platforms read "--" as an em dash and leave
// "---" alone. Taking goldmark's defaults would not close the gap, it
// would move it, and would newly break "---", which the two renderers
// agree about today.
//
// Quotes keep goldmark's defaults, which do match: straight quotes and
// apostrophes in prose curl in both. Nothing inside a code span or a
// fence is touched, since neither is inline text.
func typographer() goldmark.Extender {
	return extension.NewTypographer(
		extension.WithTypographicSubstitutions(map[extension.TypographicPunctuation][]byte{
			extension.EnDash: []byte("&mdash;"),
			// "---" substituted with itself. Disabling the rule instead
			// lets the "--" rule eat the first two hyphens and leave a
			// stray third; this consumes all three and emits them back.
			extension.EmDash: []byte("---"),
		}),
	)
}

func (s *preparedSite) markdownToHTML(markdown, filePath string) (string, error) {
	if s == nil {
		s = &preparedSite{}
	}
	cfg := s.config
	if cfg.AllowUnsafe {
		markdown = rewriteLocalHTMLAttributes(markdown, filePath, cfg.HTMLExt, cfg.Index, cfg.Format)
		markdown = preprocessHTMLBlocks(markdown)
	}

	jscfg := s.jsonSpecConfig

	highlightOpts := []highlighting.Option{
		highlighting.WithStyle("github"),
		highlighting.WithFormatOptions(
			chromahtml.WithLineNumbers(false),
			chromahtml.WithClasses(true),
		),
	}
	var inner highlighting.WrapperRenderer
	if len(jscfg.DiscriminatorPrefixes) > 0 {
		inner = jsonspec.WrapperRenderer(jscfg)
	}
	highlightOpts = append(highlightOpts,
		highlighting.WithWrapperRenderer(codeBlockWrapper(inner)),
	)

	extensions := []goldmark.Extender{
		extension.GFM,
		extension.Footnote,
		typographer(),
		meta.Meta,
		highlighting.NewHighlighting(highlightOpts...),
		&admonitions.Extender{},
		alertsExtender{},
		tabs.Extender{},
		components.Extender{Registry: s.componentsRegistry(), Icons: s.navIconType},
		media.Extender{},
		jsonspec.Extension(jscfg),
	}
	if cfg.TOC {
		extensions = append(extensions, &toc.Extender{
			MinDepth: 1,
			MaxDepth: 6,
			ListID:   "toc",
			TitleID:  "toc-title",
		})
	}

	parserOpts := []parser.Option{parser.WithAutoHeadingID()}
	if cfg.AllowUnsafe {
		parserOpts = append(parserOpts, parser.WithAttribute())
	}

	// Hard wraps are deliberately off: GFM treats a single newline as a
	// space, so prose reflows to the viewport instead of breaking at the
	// column the author happened to wrap the source at.
	rendererOpts := []renderer.Option{html.WithXHTML()}
	if cfg.AllowUnsafe {
		rendererOpts = append(rendererOpts, html.WithUnsafe())
	}

	md := goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(parserOpts...),
		goldmark.WithRendererOptions(rendererOpts...),
	)

	source := []byte(markdown)
	pc := parser.NewContext(parser.WithIDs(anchor.NewIDs()))

	if filePath != "" {
		doc := md.Parser().Parse(text.NewReader(source), parser.WithContext(pc))
		ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			link, ok := n.(*ast.Link)
			if !ok {
				return ast.WalkContinue, nil
			}
			href := string(link.Destination)
			if rewritten, ok := rewriteMarkdownReference(cfg.Format, filePath, href, cfg.HTMLExt, cfg.Index); ok {
				link.Destination = []byte(rewritten)
			}
			return ast.WalkContinue, nil
		})
		var buf bytes.Buffer
		if err := md.Renderer().Render(&buf, source, doc); err != nil {
			return "", fmt.Errorf("render markdown: %w", err)
		}
		logExtensionErrors(pc, filePath)
		return buf.String(), nil
	}

	doc := md.Parser().Parse(text.NewReader(source), parser.WithContext(pc))
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	logExtensionErrors(pc, filePath)
	return buf.String(), nil
}

func markdownToHTMLWithContext(cfg Config, markdown, filePath string) (string, error) {
	site, err := prepareSite(cfg, slog.Default())
	if err != nil {
		return "", err
	}
	return site.markdownToHTML(markdown, filePath)
}

type alertsExtender struct{}

func (alertsExtender) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithASTTransformers(
		util.Prioritized(alertsTransformer{}, 900),
	))
	md.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(alertsRenderer{}, 100),
	))
}

type alertsTransformer struct{}

func (alertsTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Kind() != ast.KindBlockquote {
			return ast.WalkContinue, nil
		}
		bq := n.(*ast.Blockquote)
		para, ok := bq.FirstChild().(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		kind, markerLen, ok := alertKind(source, para)
		if !ok {
			return ast.WalkContinue, nil
		}
		bq.SetAttribute(alertKindAttr, kind)
		stripAlertMarker(source, para, markerLen)
		if para.FirstChild() == nil {
			bq.RemoveChild(bq, para)
		}
		return ast.WalkContinue, nil
	})
}

func alertKind(source []byte, para *ast.Paragraph) ([]byte, int, bool) {
	var buf []byte
	for n := para.FirstChild(); n != nil && len(buf) < len("[!IMPORTANT]"); n = n.NextSibling() {
		txt, ok := n.(*ast.Text)
		if !ok {
			break
		}
		buf = append(buf, txt.Segment.Value(source)...)
	}
	alerts := [...]struct {
		marker string
		kind   string
	}{
		{"[!NOTE]", "note"},
		{"[!TIP]", "tip"},
		{"[!IMPORTANT]", "important"},
		{"[!WARNING]", "warning"},
		{"[!CAUTION]", "danger"},
	}
	for _, a := range alerts {
		if bytes.HasPrefix(buf, []byte(a.marker)) {
			return []byte(a.kind), len(a.marker), true
		}
	}
	return nil, 0, false
}

func stripAlertMarker(source []byte, para *ast.Paragraph, n int) {
	for c := para.FirstChild(); c != nil && n > 0; {
		next := c.NextSibling()
		txt, ok := c.(*ast.Text)
		if !ok {
			return
		}
		value := txt.Segment.Value(source)
		if n >= len(value) {
			n -= len(value)
			para.RemoveChild(para, txt)
			c = next
			continue
		}
		txt.Segment = text.NewSegment(txt.Segment.Start+n, txt.Segment.Stop)
		break
	}
	for c := para.FirstChild(); c != nil; c = c.NextSibling() {
		txt, ok := c.(*ast.Text)
		if !ok {
			return
		}
		value := txt.Segment.Value(source)
		trim := len(value) - len(bytes.TrimLeft(value, " \t"))
		if trim == 0 {
			return
		}
		if trim == len(value) {
			para.RemoveChild(para, txt)
			continue
		}
		txt.Segment = text.NewSegment(txt.Segment.Start+trim, txt.Segment.Stop)
		return
	}
}

type alertsRenderer struct{}

func (alertsRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindBlockquote, renderAlertBlockquote)
}

func renderAlertBlockquote(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	v, ok := n.Attribute(alertKindAttr)
	if !ok {
		if entering {
			if n.Attributes() != nil {
				_, _ = w.WriteString("<blockquote")
				html.RenderAttributes(w, n, html.BlockquoteAttributeFilter)
				_, _ = w.WriteString(">\n")
			} else {
				_, _ = w.WriteString("<blockquote>\n")
			}
		} else {
			_, _ = w.WriteString("</blockquote>\n")
		}
		return ast.WalkContinue, nil
	}
	kind := v.([]byte)
	if entering {
		_, _ = w.WriteString(`<div class="admonition adm-`)
		_, _ = w.Write(util.EscapeHTML(kind))
		_, _ = w.WriteString(`">`)
		_ = w.WriteByte('\n')
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

// logExtensionErrors surfaces parse diagnostics recorded by the tabs and
// components extensions. These never abort rendering; they log at warn
// level so a misformed fence or an unknown component tag is visible
// without breaking the build.
func logExtensionErrors(pc parser.Context, filePath string) {
	logger := slog.Default()
	warn := func(source string, line int, msg string) {
		attrs := []any{"line", line}
		if filePath != "" {
			attrs = append(attrs, "file", filePath)
		}
		attrs = append(attrs, "msg", msg)
		logger.Warn(source+": parse error", attrs...)
	}
	for _, e := range tabs.Errors(pc) {
		warn("tabs", e.Line, e.Msg)
	}
	for _, e := range components.Errors(pc) {
		msg := e.Msg
		if e.UnknownComponent {
			msg += " (known: " + strings.Join(e.Known, ", ") + ")"
		}
		warn("components", e.Line, msg)
	}
}

func rewriteLocalHTMLAttributes(content, filePath, htmlExt, indexFile, format string) string {
	if filePath == "" {
		return content
	}

	var out strings.Builder
	inFence := false
	fence := byte(0)
	fenceLen := 0
	for line := range strings.SplitAfterSeq(content, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if marker, n := markdownFence(trimmed); n >= 3 {
			if !inFence {
				inFence, fence, fenceLen = true, marker, n
			} else if marker == fence && n >= fenceLen {
				inFence = false
			}
			out.WriteString(line)
			continue
		}
		if inFence || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
			out.WriteString(line)
			continue
		}
		out.WriteString(rewriteHTMLAttributes(line, filePath, htmlExt, indexFile, format))
	}
	return out.String()
}

func markdownFence(line string) (byte, int) {
	if line == "" || line[0] != '`' && line[0] != '~' {
		return 0, 0
	}
	marker := line[0]
	n := 1
	for n < len(line) && line[n] == marker {
		n++
	}
	return marker, n
}

func rewriteHTMLAttributes(content, filePath, htmlExt, indexFile, format string) string {
	return htmlLinkAttrPattern.ReplaceAllStringFunc(content, func(attr string) string {
		match := htmlLinkAttrPattern.FindStringSubmatch(attr)
		if len(match) != 4 {
			return attr
		}
		raw := match[2]
		quote := `"`
		if raw == "" {
			raw = match[3]
			quote = `'`
		}
		rewritten, ok := rewriteMarkdownReference(format, filePath, raw, htmlExt, indexFile)
		if !ok {
			return attr
		}
		return match[1] + "=" + quote + rewritten + quote
	})
}

// DocumentData is the parsed Markdown document passed through rendering.
//
// Its exported fields are template-visible and follow semantic versioning.
type DocumentData struct {
	// Content is the Markdown body after YAML frontmatter has been parsed.
	// The frontmatter block itself is not included.
	Content string
	// Frontmatter is the parsed YAML frontmatter map. It is empty when
	// the document has no frontmatter or frontmatter parsing failed.
	Frontmatter map[string]any
}

func loadJSONFile(filename string) (any, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var jsonData any
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, err
	}

	return jsonData, nil
}

func parseFrontmatter(content string) (DocumentData, error) {
	// Use goldmark to parse frontmatter
	md := goldmark.New(goldmark.WithExtensions(meta.Meta))
	context := parser.NewContext(parser.WithIDs(anchor.NewIDs()))

	// Parse to extract metadata
	source := []byte(content)
	tree := md.Parser().Parse(text.NewReader(source), parser.WithContext(context))

	// Get metadata
	metaData := meta.Get(context)
	if metaData == nil {
		metaData = make(map[string]any)
	}

	return DocumentData{
		Content:     documentBody(source, tree),
		Frontmatter: metaData,
	}, nil
}

func documentBody(source []byte, tree ast.Node) string {
	var start int
	found := false
	_ = ast.Walk(tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if found || !entering || n.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		lines := n.Lines()
		if lines == nil || lines.Len() == 0 {
			return ast.WalkContinue, nil
		}
		start = lineStart(source, lines.At(0).Start)
		found = true
		return ast.WalkStop, nil
	})
	if !found {
		return ""
	}
	return string(source[start:])
}

func lineStart(source []byte, pos int) int {
	if pos > len(source) {
		pos = len(source)
	}
	for pos > 0 && source[pos-1] != '\n' {
		pos--
	}
	return pos
}

func renderFrontmatterHTML(frontmatter map[string]any) string {
	if len(frontmatter) == 0 {
		return ""
	}
	b, err := yaml.Marshal(frontmatter)
	if err != nil {
		return ""
	}
	return `<pre class="frontmatter"><code class="language-yaml">` +
		template.HTMLEscapeString(string(b)) +
		"</code></pre>\n"
}
