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
	"CardGroup": {
		Attrs: []string{"cols"},
		Template: template.Must(template.New("CardGroup").Parse(
			`<div class="md-card-group" data-cols="{{or .Attrs.cols "2"}}">{{.Content}}</div>`)),
	},
}
