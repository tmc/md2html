package components

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// render converts markdown and returns the HTML plus any parse errors.
func render(t *testing.T, markdown string) (string, []*ParseError) {
	t.Helper()
	md := goldmark.New(goldmark.WithExtensions(Extender{}))
	pc := parser.NewContext()
	source := []byte(markdown)
	doc := md.Parser().Parse(text.NewReader(source), parser.WithContext(pc))
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String(), Errors(pc)
}

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "body is markdown",
			in:   "<Card title=\"Install\">\nRun `go install`.\n</Card>\n",
			want: []string{
				`<div class="md-card">`,
				`<div class="md-card-title">Install</div>`,
				"<code>go install</code>",
				"</div>",
			},
		},
		{
			name: "href wraps the title",
			in:   "<Card title=\"Go\" href=\"/go\">\ntext\n</Card>\n",
			want: []string{`<a class="md-card-link" href="/go">`},
		},
		{
			name: "brace expression",
			in:   "<CardGroup cols={3}>\n<Card title=\"A\">\na\n</Card>\n</CardGroup>\n",
			want: []string{`data-cols="3"`, `<div class="md-card">`},
		},
		{
			name: "cols defaults",
			in:   "<CardGroup>\n<Card title=\"A\">\na\n</Card>\n</CardGroup>\n",
			want: []string{`data-cols="2"`},
		},
		{
			name: "self closing",
			in:   "<Card title=\"Empty\"/>\n",
			want: []string{`<div class="md-card-body"></div>`},
		},
		{
			name: "nested same name",
			in:   "<Card title=\"outer\">\n<Card title=\"inner\">\nx\n</Card>\n</Card>\n",
			want: []string{"outer", "inner"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errs := render(t, tt.in)
			if len(errs) > 0 {
				t.Fatalf("unexpected parse errors: %v", errs)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

func TestNestedSameNameNesting(t *testing.T) {
	got, errs := render(t, "<Card title=\"outer\">\n<Card title=\"inner\">\nx\n</Card>\n</Card>\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	// The inner card must be nested inside the outer card's body, not
	// be a sibling left over from a mismatched close.
	body := `<div class="md-card-body">`
	if n := strings.Count(got, body); n != 2 {
		t.Fatalf("want 2 card bodies, got %d\n%s", n, got)
	}
	outer := strings.Index(got, "outer")
	inner := strings.Index(got, "inner")
	if outer < 0 || inner < outer {
		t.Fatalf("inner card not nested inside outer\n%s", got)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"unknown component", "<Widget/>\n", "unknown component <Widget>"},
		{"missing required", "<Card/>\n", `missing required attribute "title"`},
		{"unknown attribute", "<Card title=\"a\" titel=\"b\"/>\n", `no attribute "titel"`},
		{"unclosed", "<Card title=\"a\">\nbody\n", "unclosed <Card>"},
		{"non-scalar expression", "<CardGroup cols={n + 1}>\n</CardGroup>\n", "not a JSON scalar"},
		{"unterminated quote", "<Card title=\"a>\n</Card>\n", "unterminated"},
		{"duplicate attribute", "<Card title=\"a\" title=\"b\"/>\n", "duplicate attribute"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := render(t, tt.in)
			if len(errs) == 0 {
				t.Fatalf("want an error containing %q, got none", tt.want)
			}
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e.Msg)
			}
			if !strings.Contains(strings.Join(msgs, "\n"), tt.want) {
				t.Errorf("want error containing %q, got %v", tt.want, msgs)
			}
		})
	}
}

// TestAttributesAreEscaped guards the reason components use
// html/template rather than string concatenation.
func TestAttributesAreEscaped(t *testing.T) {
	got, errs := render(t, "<Card title=\"&lt;script&gt;\" href=\"javascript:alert(1)\">\nx\n</Card>\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if strings.Contains(got, "javascript:alert(1)") {
		t.Errorf("javascript: URL was not filtered\n%s", got)
	}
	if strings.Contains(got, "<script>") {
		t.Errorf("title was not escaped\n%s", got)
	}
}

// TestLowercaseTagsIgnored keeps ordinary HTML flowing to goldmark's own
// HTML block parser.
func TestLowercaseTagsIgnored(t *testing.T) {
	_, errs := render(t, "<div>\nhello\n</div>\n")
	if len(errs) > 0 {
		t.Fatalf("lowercase tags must not be treated as components: %v", errs)
	}
}

func TestTemplatesReferenceContentOnce(t *testing.T) {
	for _, name := range DefaultRegistry.Names() {
		comp, _ := DefaultRegistry.Lookup(name)
		var buf bytes.Buffer
		data := Data{Attrs: map[string]string{}, Content: contentPlaceholder}
		if err := comp.Template.Execute(&buf, data); err != nil {
			t.Fatalf("%s: execute: %v", name, err)
		}
		if n := strings.Count(buf.String(), contentPlaceholder); n != 1 {
			t.Errorf("%s: template references .Content %d times, want 1", name, n)
		}
	}
}

func TestBuiltinComponents(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{
			name: "steps are an ordered list",
			in:   "<Steps>\n<Step title=\"First\">\ndo a thing\n</Step>\n<Step title=\"Second\" stepNumber=\"7\">\nthen this\n</Step>\n</Steps>\n",
			want: []string{
				`<ol class="md-steps" data-title-size="p">`,
				`<li class="md-step">`,
				`<div class="md-step-title">First</div>`,
				`<li class="md-step" value="7">`,
				"<p>do a thing</p>",
			},
		},
		{
			name: "steps title size",
			in:   "<Steps titleSize=\"h3\">\n<Step title=\"A\">\nx\n</Step>\n</Steps>\n",
			want: []string{`data-title-size="h3"`},
		},
		{
			name: "accordion is a details element",
			in:   "<Accordion title=\"More\" description=\"detail\">\nhidden *body*\n</Accordion>\n",
			want: []string{
				`<details class="md-accordion">`,
				`<span class="md-accordion-title">More</span>`,
				`<span class="md-accordion-description">detail</span>`,
				"<em>body</em>",
			},
			absent: []string{" open"},
		},
		{
			name:   "defaultOpen false stays closed",
			in:     "<Accordion title=\"A\" defaultOpen=\"false\">\nx\n</Accordion>\n",
			absent: []string{" open>"},
		},
		{
			name: "defaultOpen true opens",
			in:   "<Accordion title=\"A\" defaultOpen={true}>\nx\n</Accordion>\n",
			want: []string{" open>"},
		},
		{
			name: "bare defaultOpen opens",
			in:   "<Accordion title=\"A\" defaultOpen>\nx\n</Accordion>\n",
			want: []string{" open>"},
		},
		{
			name: "expandable is a disclosure too",
			in:   "<Expandable title=\"child fields\">\nx\n</Expandable>\n",
			want: []string{`<details class="md-accordion">`, "child fields"},
		},
		{
			name: "accordion group wraps",
			in:   "<AccordionGroup>\n<Accordion title=\"A\">\nx\n</Accordion>\n</AccordionGroup>\n",
			want: []string{`<div class="md-accordion-group">`, `<details class="md-accordion">`},
		},
		{
			name: "frame captions",
			in:   "<Frame caption=\"Below\" hint=\"Above\">\n![alt](x.png)\n</Frame>\n",
			want: []string{
				`<figure class="md-frame">`,
				`<div class="md-frame-hint">Above</div>`,
				`<figcaption class="md-frame-caption">Below</figcaption>`,
			},
		},
		{
			name: "columns is an alias of card group",
			in:   "<Columns cols={4}>\n<Card title=\"A\">\na\n</Card>\n</Columns>\n",
			want: []string{`<div class="md-card-group" data-cols="4">`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errs := render(t, tt.in)
			if len(errs) > 0 {
				t.Fatalf("unexpected parse errors: %v", errs)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(got, absent) {
					t.Errorf("output unexpectedly contains %q\ngot:\n%s", absent, got)
				}
			}
		})
	}
}

func TestDataBool(t *testing.T) {
	d := Data{Attrs: map[string]string{"a": "true", "b": "TRUE", "c": "1", "d": "false", "e": "", "f": "no"}}
	for _, name := range []string{"a", "b", "c"} {
		if !d.Bool(name) {
			t.Errorf("Bool(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"d", "e", "f", "missing"} {
		if d.Bool(name) {
			t.Errorf("Bool(%q) = true, want false", name)
		}
	}
}
