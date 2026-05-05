// Package media provides a goldmark extension that rewrites image
// syntax (![alt](url)) as <video> or <audio> elements when the URL
// points at a media file. Other images fall through to the normal
// <img> rendering.
package media

import (
	"path"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// Extender registers the media renderer on a goldmark Markdown instance.
type Extender struct{}

// Extend implements goldmark.Extender.
func (Extender) Extend(md goldmark.Markdown) {
	md.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&Renderer{}, 100),
		),
	)
}

// Renderer overrides KindImage so URLs ending in known media extensions
// emit <video>/<audio> instead of <img>.
type Renderer struct {
	Unsafe bool
	XHTML  bool
}

// RegisterFuncs implements renderer.NodeRenderer.
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, r.renderImage)
}

// The html package keeps its option-name constants unexported, so we
// match by the raw string values it registers them under.
const (
	optUnsafe renderer.OptionName = "Unsafe"
	optXHTML  renderer.OptionName = "XHTML"
)

// SetOption picks up html.WithUnsafe / html.WithXHTML so our <img>
// fallback matches the default renderer's behavior.
func (r *Renderer) SetOption(name renderer.OptionName, value any) {
	switch name {
	case optUnsafe:
		if b, ok := value.(bool); ok {
			r.Unsafe = b
		}
	case optXHTML:
		if b, ok := value.(bool); ok {
			r.XHTML = b
		}
	}
}

var videoExts = map[string]bool{
	".mp4":  true,
	".webm": true,
	".mov":  true,
	".m4v":  true,
	".ogv":  true,
}

var audioExts = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".oga":  true,
	".ogg":  true,
	".m4a":  true,
	".flac": true,
	".aac":  true,
}

func kind(dest []byte) string {
	// Strip query/fragment before extension check so links like
	// foo.mp4?t=1 still resolve.
	s := string(dest)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	ext := strings.ToLower(path.Ext(s))
	if videoExts[ext] {
		return "video"
	}
	if audioExts[ext] {
		return "audio"
	}
	return ""
}

// IsMediaPath reports whether name has an audio or video extension
// recognized by Extender.
func IsMediaPath(name string) bool {
	return kind([]byte(name)) != ""
}

func (r *Renderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	tag := kind(n.Destination)
	if tag == "" {
		return r.renderImg(w, source, n)
	}

	safeDest := n.Destination
	if !r.Unsafe && html.IsDangerousURL(n.Destination) {
		safeDest = nil
	}

	_, _ = w.WriteString("<")
	_, _ = w.WriteString(tag)
	_, _ = w.WriteString(` src="`)
	_, _ = w.Write(util.EscapeHTML(util.URLEscape(safeDest, true)))
	_, _ = w.WriteString(`" controls`)
	if n.Title != nil {
		_, _ = w.WriteString(` title="`)
		_, _ = w.Write(util.EscapeHTML(n.Title))
		_ = w.WriteByte('"')
	}
	_ = w.WriteByte('>')
	// Fallback text for user agents that don't support the element.
	alt := altText(source, n)
	if alt != "" {
		_, _ = w.Write(util.EscapeHTML([]byte(alt)))
	}
	_, _ = w.WriteString("</")
	_, _ = w.WriteString(tag)
	_ = w.WriteByte('>')
	return ast.WalkSkipChildren, nil
}

// renderImg mirrors the default goldmark <img> rendering so non-media
// images still work when this renderer wins the KindImage slot.
func (r *Renderer) renderImg(w util.BufWriter, source []byte, n *ast.Image) (ast.WalkStatus, error) {
	_, _ = w.WriteString(`<img src="`)
	if r.Unsafe || !html.IsDangerousURL(n.Destination) {
		_, _ = w.Write(util.EscapeHTML(util.URLEscape(n.Destination, true)))
	}
	_, _ = w.WriteString(`" alt="`)
	_, _ = w.Write(util.EscapeHTML([]byte(altText(source, n))))
	_ = w.WriteByte('"')
	if n.Title != nil {
		_, _ = w.WriteString(` title="`)
		_, _ = w.Write(util.EscapeHTML(n.Title))
		_ = w.WriteByte('"')
	}
	if n.Attributes() != nil {
		html.RenderAttributes(w, n, html.ImageAttributeFilter)
	}
	if r.XHTML {
		_, _ = w.WriteString(" />")
	} else {
		_ = w.WriteByte('>')
	}
	return ast.WalkSkipChildren, nil
}

func altText(source []byte, n ast.Node) string {
	var b strings.Builder
	collectText(&b, source, n)
	return b.String()
}

func collectText(b *strings.Builder, source []byte, n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.String:
			b.Write(t.Value)
		case *ast.Text:
			b.Write(t.Segment.Value(source))
		default:
			collectText(b, source, c)
		}
	}
}
