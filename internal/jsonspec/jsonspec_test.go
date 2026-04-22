package jsonspec

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

func newMarkdown(cfg Config) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			highlighting.NewHighlighting(
				highlighting.WithWrapperRenderer(WrapperRenderer(cfg)),
			),
			Extension(cfg),
		),
	)
}

func TestRender(t *testing.T) {
	cfg := Config{
		DiscriminatorPrefixes: []string{"ascf/"},
		BadgeURLTemplate:      "schemas.html#%s",
	}
	md := newMarkdown(cfg)

	tests := []struct {
		name    string
		input   string
		want    []string
		notWant []string
	}{
		{
			name: "discriminator detected",
			input: "```json\n" +
				`{"type": "ascf/hypothesis", "id": "h1"}` + "\n```\n",
			want: []string{
				`<div class="md-jsonspec" data-schema-type="hypothesis">`,
				`class="md-jsonspec-badge" href="schemas.html#hypothesis"`,
				`hypothesis</a>`,
			},
		},
		{
			name: "no discriminator leaves block untouched",
			input: "```json\n" +
				`{"id": "h1"}` + "\n```\n",
			notWant: []string{
				`md-jsonspec`,
				`data-schema-type`,
			},
		},
		{
			name: "non-ascf discriminator ignored",
			input: "```json\n" +
				`{"type": "example/foo"}` + "\n```\n",
			notWant: []string{`md-jsonspec`},
		},
		{
			name: "non-json fence ignored",
			input: "```go\n" +
				`{"type": "ascf/hypothesis"}` + "\n```\n",
			notWant: []string{`md-jsonspec`},
		},
		{
			name: "spaces around colon tolerated",
			input: "```json\n" +
				`{  "type"  :  "ascf/island"  }` + "\n```\n",
			want: []string{`data-schema-type="island"`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := md.Convert([]byte(tc.input), &buf); err != nil {
				t.Fatalf("convert: %v", err)
			}
			out := buf.String()
			for _, s := range tc.want {
				if !strings.Contains(out, s) {
					t.Errorf("output missing %q\n---\n%s", s, out)
				}
			}
			for _, s := range tc.notWant {
				if strings.Contains(out, s) {
					t.Errorf("output unexpectedly contains %q\n---\n%s", s, out)
				}
			}
		})
	}
}

func TestNoPrefixesIsNoOp(t *testing.T) {
	md := newMarkdown(Config{})
	var buf bytes.Buffer
	in := "```json\n" + `{"type": "ascf/hypothesis"}` + "\n```\n"
	if err := md.Convert([]byte(in), &buf); err != nil {
		t.Fatalf("convert: %v", err)
	}
	if strings.Contains(buf.String(), "md-jsonspec") {
		t.Errorf("expected no wrapper when DiscriminatorPrefixes empty\n%s", buf.String())
	}
}

// TestPrefixSeparatorTolerance exercises the namespace-separator trim
// applied after a discriminator prefix is cut: "ascf" as a prefix must
// produce the same suffix as "ascf/" for the value "ascf/challenge".
func TestPrefixSeparatorTolerance(t *testing.T) {
	cases := []struct {
		prefix string
		value  string
		want   string
	}{
		{"ascf/", "ascf/challenge", "challenge"},
		{"ascf", "ascf/challenge", "challenge"},
		{"ascf:", "ascf:island", "island"},
		{"ascf", "ascf:island", "island"},
		{"ascf", "ascf.island", "island"},
		{"ascf", "ascf-island", "island"},
	}
	for _, tc := range cases {
		t.Run(tc.prefix+"_"+tc.value, func(t *testing.T) {
			md := newMarkdown(Config{
				DiscriminatorPrefixes: []string{tc.prefix},
				BadgeURLTemplate:      "schemas.html#%s",
			})
			var buf bytes.Buffer
			in := "```json\n{\"type\": \"" + tc.value + "\"}\n```\n"
			if err := md.Convert([]byte(in), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if !strings.Contains(out, `data-schema-type="`+tc.want+`"`) {
				t.Errorf("want data-schema-type=%q, got:\n%s", tc.want, out)
			}
			if !strings.Contains(out, `href="schemas.html#`+tc.want+`"`) {
				t.Errorf("want badge url suffix %q, got:\n%s", tc.want, out)
			}
		})
	}
}

func TestLabelTemplate(t *testing.T) {
	cfg := Config{
		DiscriminatorPrefixes: []string{"ascf/"},
		BadgeURLTemplate:      "schemas.html#%s",
		BadgeLabelTemplate:    "\u00A0%s schema",
	}
	md := newMarkdown(cfg)
	var buf bytes.Buffer
	in := "```json\n" + `{"type": "ascf/hypothesis"}` + "\n```\n"
	if err := md.Convert([]byte(in), &buf); err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(buf.String(), "hypothesis schema</a>") {
		t.Errorf("expected label template to apply\n%s", buf.String())
	}
}
