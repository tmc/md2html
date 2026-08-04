package md2html

import (
	"strings"
	"testing"
)

func TestPageHasMath(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{name: "inline dollars", html: "<p>Inline $a_i + b_j$ math.</p>", want: true},
		{name: "display dollars", html: "<p>$$E = mc^2$$</p>", want: true},
		{name: "inline parens", html: `<p>\(x_1\)</p>`, want: true},
		{name: "display brackets", html: `<p>\[x_1\]</p>`, want: true},
		{name: "no math", html: "<p>plain prose</p>", want: false},
		{name: "dollars only in code", html: `<pre><code>echo "$a and $b"</code></pre>`, want: false},
		{name: "dollars only in inline code", html: `<p>run <code>$PATH:$HOME</code> now</p>`, want: false},
		{name: "math beside code", html: `<p>$x^2$ and <code>$HOME</code></p>`, want: true},
		{name: "currency amounts", html: "<p>cost over $3 billion; consumers paid $1000 and later $100.</p>", want: false},
		{name: "currency pair mid-sentence", html: "<p>between $5 and $10 per unit</p>", want: false},
		{name: "math at end of sentence", html: "<p>bounded by $\\rho(Dg) < 1$.</p>", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pageHasMath(tt.html); got != tt.want {
				t.Fatalf("pageHasMath(%q) = %v, want %v", tt.html, got, tt.want)
			}
		})
	}
}

func TestRenderTemplateLoadsMathJaxOnlyForMath(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "plain", content: "<p>plain prose</p>", want: false},
		{name: "math", content: "<p>$x^2$</p>", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := mustRenderTemplate(t, Config{}, tt.content, "test", "", false, nil)
			got := strings.Contains(html, `id="MathJax-script"`)
			if got != tt.want {
				t.Fatalf("MathJax included = %v, want %v:\n%s", got, tt.want, html)
			}
		})
	}
}

// TestMathJaxConfigPrecedesLoader checks that window.MathJax is assigned
// before the loader script tag. MathJax 3 reads the configuration as it
// loads, so the reverse order makes the delimiter setup a race.
func TestMathJaxConfigPrecedesLoader(t *testing.T) {
	got := mustRenderTemplate(t, Config{}, "<p>$x$</p>", "Math", "", false, nil)
	config := strings.Index(got, "window.MathJax = {")
	loader := strings.Index(got, `id="MathJax-script"`)
	if config < 0 || loader < 0 {
		t.Fatalf("MathJax scripts missing: config=%d loader=%d", config, loader)
	}
	if config > loader {
		t.Errorf("window.MathJax is assigned after the loader (config=%d, loader=%d)", config, loader)
	}
}

// TestCDNAssetsReportFailure checks that the CDN-loaded scripts announce a
// load failure instead of silently leaving diagrams and math unrendered.
func TestCDNAssetsReportFailure(t *testing.T) {
	got := mustRenderTemplate(t, Config{}, "<p>$x$</p>", "Math", "", false, nil)
	for _, want := range []string{
		`onerror="md2htmlAssetUnavailable('MathJax'`,
		`onerror="md2htmlAssetUnavailable('Mermaid'`,
		"window.md2htmlAssetUnavailable = window.md2htmlAssetUnavailable ||",
		"if (typeof mermaid === 'undefined')",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
}
