package components

import (
	"fmt"
	"html/template"
	"slices"
	"strings"
)

// A Component is the contract for one registered component name.
type Component struct {
	// Attrs lists every attribute the component accepts. An attribute
	// outside this list is a parse error, which turns a typo into a
	// diagnostic instead of a silently dropped value.
	Attrs []string

	// Required lists the attributes that must be present. Every name
	// here must also appear in Attrs.
	Required []string

	// Template renders the component. It is executed with [Data], and
	// must reference .Content exactly once so that the Markdown body
	// has a single home in the output.
	Template *template.Template

	// Inline marks a component that appears within a line of text
	// rather than standing alone as a block, such as a badge. Its body
	// is inline Markdown, so it cannot contain paragraphs or lists.
	Inline bool
}

// Data is the value passed to a component's template.
type Data struct {
	// Attrs holds the attribute values written on the tag. Absent
	// attributes are missing from the map, so {{.Attrs.title}} renders
	// as the empty string and is falsy in {{if}} and {{with}}.
	Attrs map[string]string

	// Content is the rendered Markdown body of the component.
	Content template.HTML
}

// Bool reports whether the named attribute is set to a true value.
// Templates need this because every attribute is a string, and a bare
// {{if .Attrs.defaultOpen}} would treat defaultOpen="false" as true.
func (d Data) Bool(name string) bool {
	switch strings.ToLower(d.Attrs[name]) {
	case "true", "yes", "1":
		return true
	}
	return false
}

// A Registry maps component names to their contracts.
type Registry map[string]Component

// Lookup returns the component registered under name.
func (r Registry) Lookup(name string) (Component, bool) {
	c, ok := r[name]
	return c, ok
}

// Names returns the registered component names in sorted order.
func (r Registry) Names() []string {
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// validate checks attrs against the component's contract, returning one
// message per problem so that a tag with several mistakes reports all of
// them rather than only the first.
func (c Component) validate(name string, attrs map[string]string) []string {
	var msgs []string
	for _, req := range c.Required {
		if _, ok := attrs[req]; !ok {
			msgs = append(msgs, fmt.Sprintf("<%s> is missing required attribute %q", name, req))
		}
	}
	unknown := make([]string, 0, len(attrs))
	for attr := range attrs {
		if !slices.Contains(c.Attrs, attr) {
			unknown = append(unknown, attr)
		}
	}
	slices.Sort(unknown)
	for _, attr := range unknown {
		msgs = append(msgs, fmt.Sprintf("<%s> has no attribute %q (accepts: %s)",
			name, attr, strings.Join(c.Attrs, ", ")))
	}
	return msgs
}

// DefaultRegistry holds the components bundled with md2html. It is
// deliberately fixed: rendering an unregistered tag would mean guessing
// at markup the theme has no styles for.
var DefaultRegistry = Registry{
	"Card": {
		Attrs:    []string{"title", "icon", "href"},
		Required: []string{"title"},
		Template: template.Must(template.New("Card").Parse(
			`<div class="md-card">` +
				`{{if .Attrs.href}}<a class="md-card-link" href="{{.Attrs.href}}">{{end}}` +
				`<div class="md-card-title">` +
				`{{with .Attrs.icon}}<span class="md-card-icon" aria-hidden="true">{{.}}</span>{{end}}` +
				`{{.Attrs.title}}</div>` +
				`{{if .Attrs.href}}</a>{{end}}` +
				`<div class="md-card-body">{{.Content}}</div>` +
				`</div>`)),
	},
	"CardGroup": cardGroup,

	// Columns and CardGroup are two spellings of the same card grouping
	// wrapper, both current in MDX documentation. Accepting either means
	// a document does not have to be rewritten to render.
	"Columns": cardGroup,

	"Steps": {
		Attrs: []string{"titleSize"},
		Template: template.Must(template.New("Steps").Parse(
			`<ol class="md-steps" data-title-size="{{or .Attrs.titleSize "p"}}">{{.Content}}</ol>`)),
	},
	"Step": {
		Attrs:    []string{"title", "icon", "stepNumber", "id", "noAnchor"},
		Required: []string{"title"},
		Template: template.Must(template.New("Step").Parse(
			`<li class="md-step"{{with .Attrs.stepNumber}} value="{{.}}"{{end}}` +
				`{{with .Attrs.id}} id="{{.}}"{{end}}>` +
				`<div class="md-step-title">` +
				`{{with .Attrs.icon}}<span class="md-step-icon" aria-hidden="true">{{.}}</span>{{end}}` +
				`{{.Attrs.title}}</div>` +
				`<div class="md-step-body">{{.Content}}</div>` +
				`</li>`)),
	},

	"AccordionGroup": {
		Template: template.Must(template.New("AccordionGroup").Parse(
			`<div class="md-accordion-group">{{.Content}}</div>`)),
	},
	"Accordion":  accordion("Accordion"),
	"Expandable": accordion("Expandable"),

	"Badge": {
		Attrs:  []string{"color", "size", "shape", "icon", "stroke", "disabled"},
		Inline: true,
		Template: template.Must(template.New("Badge").Parse(
			`<span class="md-badge" data-color="{{or .Attrs.color "gray"}}"` +
				` data-size="{{or .Attrs.size "sm"}}" data-shape="{{or .Attrs.shape "rounded"}}"` +
				`{{if .Bool "stroke"}} data-stroke="true"{{end}}` +
				`{{if .Bool "disabled"}} data-disabled="true"{{end}}>` +
				`{{with .Attrs.icon}}<span class="md-badge-icon" aria-hidden="true">{{.}}</span>{{end}}` +
				`{{.Content}}</span>`)),
	},
	// Tooltip uses the title attribute so the hint is available without
	// JavaScript, and to assistive technology, rather than being drawn
	// by a script that may not run.
	"Tooltip": {
		Attrs:    []string{"tip", "cta", "href"},
		Required: []string{"tip"},
		Inline:   true,
		Template: template.Must(template.New("Tooltip").Parse(
			`{{if .Attrs.href}}<a class="md-tooltip" href="{{.Attrs.href}}" title="{{.Attrs.tip}}">` +
				`{{else}}<span class="md-tooltip" title="{{.Attrs.tip}}">{{end}}` +
				`{{.Content}}` +
				`{{with .Attrs.cta}}<span class="md-tooltip-cta">{{.}}</span>{{end}}` +
				`{{if .Attrs.href}}</a>{{else}}</span>{{end}}`)),
	},

	"Note":    admonition("note"),
	"Tip":     admonition("tip"),
	"Info":    admonition("note"),
	"Warning": admonition("warning"),
	"Check":   admonition("tip"),
	"Danger":  admonition("danger"),

	"Frame": {
		Attrs: []string{"caption", "hint"},
		Template: template.Must(template.New("Frame").Parse(
			`<figure class="md-frame">` +
				`{{with .Attrs.hint}}<div class="md-frame-hint">{{.}}</div>{{end}}` +
				`<div class="md-frame-body">{{.Content}}</div>` +
				`{{with .Attrs.caption}}<figcaption class="md-frame-caption">{{.}}</figcaption>{{end}}` +
				`</figure>`)),
	},
}

// cardGroup backs both CardGroup and Columns.
var cardGroup = Component{
	Attrs: []string{"cols"},
	Template: template.Must(template.New("CardGroup").Parse(
		`<div class="md-card-group" data-cols="{{or .Attrs.cols "2"}}">{{.Content}}</div>`)),
}

// accordion builds a disclosure component. It renders as <details>, so
// expanding and collapsing works without JavaScript and a printed page
// or a reader with scripts blocked still shows the open sections.
func accordion(name string) Component {
	return Component{
		Attrs:    []string{"title", "description", "defaultOpen", "id", "icon"},
		Required: []string{"title"},
		Template: template.Must(template.New(name).Parse(
			`<details class="md-accordion"{{with .Attrs.id}} id="{{.}}"{{end}}` +
				`{{if .Bool "defaultOpen"}} open{{end}}>` +
				`<summary class="md-accordion-summary">` +
				`{{with .Attrs.icon}}<span class="md-accordion-icon" aria-hidden="true">{{.}}</span>{{end}}` +
				`<span class="md-accordion-title">{{.Attrs.title}}</span>` +
				`{{with .Attrs.description}}<span class="md-accordion-description">{{.}}</span>{{end}}` +
				`</summary>` +
				`<div class="md-accordion-body">{{.Content}}</div>` +
				`</details>`)),
	}
}

// admonition builds a callout component rendering the markup blockquote
// alerts already produce, so "> [!WARNING]" and <Warning> are styled by
// one set of rules.
//
// Info and Check have no alert kind of their own: they borrow note and
// tip, the kinds closest in meaning, rather than introduce styling for a
// distinction the stylesheet does not draw.
func admonition(kind string) Component {
	return Component{
		Template: template.Must(template.New("admonition-" + kind).Parse(
			`<div class="admonition adm-` + kind + `">` + "\n" + `{{.Content}}</div>` + "\n")),
	}
}
