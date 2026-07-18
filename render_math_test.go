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
			html := renderTemplate(Config{}, tt.content, "test", "", false, nil)
			got := strings.Contains(html, `id="MathJax-script"`)
			if got != tt.want {
				t.Fatalf("MathJax included = %v, want %v:\n%s", got, tt.want, html)
			}
		})
	}
}
