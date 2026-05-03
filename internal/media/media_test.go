package media

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

func render(t *testing.T, src string, opts ...goldmark.Option) string {
	t.Helper()
	all := append([]goldmark.Option{goldmark.WithExtensions(Extender{})}, opts...)
	md := goldmark.New(all...)
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return buf.String()
}

func TestVideoElement(t *testing.T) {
	out := render(t, "![demo](run.mp4)\n")
	for _, want := range []string{
		`<video src="run.mp4" controls>`,
		`demo</video>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
	if strings.Contains(out, "<img") {
		t.Errorf("video ref should not render as <img>:\n%s", out)
	}
}

func TestAudioElement(t *testing.T) {
	out := render(t, "![clip](song.mp3)\n")
	if !strings.Contains(out, `<audio src="song.mp3" controls>`) {
		t.Errorf("expected <audio>, got:\n%s", out)
	}
	if !strings.Contains(out, "clip</audio>") {
		t.Errorf("expected alt fallback, got:\n%s", out)
	}
}

func TestImageFallthrough(t *testing.T) {
	out := render(t, "![pic](foo.png)\n")
	if !strings.Contains(out, `<img src="foo.png"`) {
		t.Errorf("expected <img>, got:\n%s", out)
	}
	if !strings.Contains(out, `alt="pic"`) {
		t.Errorf("expected alt attr, got:\n%s", out)
	}
}

func TestXHTMLFallback(t *testing.T) {
	out := render(t, "![pic](foo.png)\n", goldmark.WithRendererOptions(html.WithXHTML()))
	if !strings.Contains(out, `<img src="foo.png" alt="pic" />`) {
		t.Errorf("expected self-closing <img />, got:\n%s", out)
	}
}

func TestQueryStringExt(t *testing.T) {
	out := render(t, "![demo](run.mp4?t=30)\n")
	if !strings.Contains(out, "<video") {
		t.Errorf("query-suffixed .mp4 should still be video:\n%s", out)
	}
}

func TestTitleAttr(t *testing.T) {
	out := render(t, `![demo](run.mp4 "my title")`+"\n")
	if !strings.Contains(out, `title="my title"`) {
		t.Errorf("expected title attr:\n%s", out)
	}
}

func TestDangerousURLBlocked(t *testing.T) {
	out := render(t, "![x](javascript:alert(1).mp4)\n")
	if strings.Contains(out, "javascript:") {
		t.Errorf("dangerous URL leaked:\n%s", out)
	}
}

func TestDangerousURLAllowedWithUnsafe(t *testing.T) {
	out := render(t, "![x](javascript:alert(1).mp4)\n", goldmark.WithRendererOptions(html.WithUnsafe()))
	if !strings.Contains(out, "javascript:") {
		t.Errorf("unsafe mode should preserve URL:\n%s", out)
	}
}
