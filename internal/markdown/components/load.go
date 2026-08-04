package components

import (
	"encoding/json"
	"fmt"
	"html/template"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// manifest is the on-disk form of a component directory, read from
// components.json.
type manifest struct {
	Components map[string]struct {
		Attrs    []string `json:"attrs"`
		Required []string `json:"required"`
		Template string   `json:"template"`
	} `json:"components"`
}

// LoadRegistry reads component definitions from dir and returns a
// registry holding them alongside the built-in components. A definition
// naming a built-in replaces it, so a site can supply its own Card
// markup.
//
// Every template is parsed and checked at load time, so a broken
// component fails while reading configuration rather than part way
// through a page.
func LoadRegistry(dir string) (Registry, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "components.json"))
	if err != nil {
		return nil, fmt.Errorf("read components config: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse components config: %w", err)
	}

	reg := Registry{}
	maps.Copy(reg, DefaultRegistry)
	for _, name := range slices.Sorted(maps.Keys(m.Components)) {
		def := m.Components[name]
		if !isName(name) {
			return nil, fmt.Errorf("component %q: name must start with an uppercase letter and contain only letters and digits", name)
		}
		if def.Template == "" {
			return nil, fmt.Errorf("component %q: missing template", name)
		}
		for _, req := range def.Required {
			if !slices.Contains(def.Attrs, req) {
				return nil, fmt.Errorf("component %q: required attribute %q is not listed in attrs", name, req)
			}
		}
		path := filepath.Join(dir, def.Template)
		text, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("component %q: %w", name, err)
		}
		tmpl, err := template.New(name).Parse(string(text))
		if err != nil {
			return nil, fmt.Errorf("component %q: parse %s: %w", name, def.Template, err)
		}
		if err := checkContent(tmpl); err != nil {
			return nil, fmt.Errorf("component %q: %s: %w", name, def.Template, err)
		}
		reg[name] = Component{
			Attrs:    def.Attrs,
			Required: def.Required,
			Template: tmpl,
		}
	}
	return reg, nil
}

// checkContent verifies that tmpl gives the component body exactly one
// home. The renderer splits the executed template around .Content, so
// zero or several references have no sensible rendering.
func checkContent(tmpl *template.Template) error {
	var buf strings.Builder
	data := Data{Attrs: map[string]string{}, Content: contentPlaceholder}
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	switch strings.Count(buf.String(), contentPlaceholder) {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("template does not reference {{.Content}} unconditionally, so the component body would be dropped")
	default:
		return fmt.Errorf("template references {{.Content}} more than once")
	}
}
