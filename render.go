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
	"github.com/tmc/md2html/internal/jsonspec"
	"github.com/tmc/md2html/internal/media"
	"github.com/tmc/md2html/internal/tabs"
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

	var buf bytes.Buffer

	// Write dark theme CSS with media query
	buf.WriteString("@media (prefers-color-scheme: dark) {\n")
	if err := formatter.WriteCSS(&buf, darkStyle); err != nil {
		return ""
	}
	buf.WriteString("\n}\n")

	// Write light theme CSS with media query (and as default)
	buf.WriteString("\n@media (prefers-color-scheme: light), (prefers-color-scheme: no-preference) {\n")
	if err := formatter.WriteCSS(&buf, lightStyle); err != nil {
		return ""
	}
	buf.WriteString("\n}\n")

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

func markdownToHTMLWithContext(cfg Config, markdown, filePath string) (string, error) {
	if cfg.AllowUnsafe {
		markdown = rewriteLocalHTMLAttributes(markdown, filePath, cfg.HTMLExt, cfg.Index, cfg.Format)
		markdown = preprocessHTMLBlocks(markdown)
	}

	jscfg := jsonSpecConfig(cfg)

	highlightOpts := []highlighting.Option{
		highlighting.WithStyle("github"),
		highlighting.WithFormatOptions(
			chromahtml.WithLineNumbers(false),
			chromahtml.WithClasses(true),
		),
	}
	if len(jscfg.DiscriminatorPrefixes) > 0 {
		highlightOpts = append(highlightOpts,
			highlighting.WithWrapperRenderer(jsonspec.WrapperRenderer(jscfg)),
		)
	}

	extensions := []goldmark.Extender{
		extension.GFM,
		extension.Footnote,
		meta.Meta,
		highlighting.NewHighlighting(highlightOpts...),
		&admonitions.Extender{},
		alertsExtender{},
		tabs.Extender{},
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

	rendererOpts := []renderer.Option{html.WithXHTML(), html.WithHardWraps()}
	if cfg.AllowUnsafe {
		rendererOpts = append(rendererOpts, html.WithUnsafe())
	}

	md := goldmark.New(
		goldmark.WithExtensions(extensions...),
		goldmark.WithParserOptions(parserOpts...),
		goldmark.WithRendererOptions(rendererOpts...),
	)

	source := []byte(markdown)
	pc := parser.NewContext()

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
		logTabsErrors(pc, filePath)
		return buf.String(), nil
	}

	doc := md.Parser().Parse(text.NewReader(source), parser.WithContext(pc))
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	logTabsErrors(pc, filePath)
	return buf.String(), nil
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

func jsonSpecConfig(cfg Config) jsonspec.Config {
	return cfg.jsonSpecConfig
}

// logTabsErrors surfaces parse diagnostics recorded by the tabs
// extension. Tab parse errors never abort rendering; they log at warn
// level so misformed fences are visible without breaking the build.
func logTabsErrors(pc parser.Context, filePath string) {
	errs := tabs.Errors(pc)
	if len(errs) == 0 {
		return
	}
	logger := slog.Default()
	for _, e := range errs {
		attrs := []any{"line", e.Line}
		if filePath != "" {
			attrs = append(attrs, "file", filePath)
		}
		attrs = append(attrs, "msg", e.Msg)
		logger.Warn("tabs: parse error", attrs...)
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
	context := parser.NewContext()

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
