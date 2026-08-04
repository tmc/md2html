package jsonspec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Bundle is a compact projection of a set of JSON Schema documents,
// keyed by discriminator suffix. It is what the client-side script
// consumes to render tooltips; field metadata it does not need is
// dropped at load time so the inline payload stays small.
type Bundle struct {
	Schemas map[string]*SchemaEntry `json:"schemas"`
}

// SchemaEntry describes a single schema.
type SchemaEntry struct {
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Required    []string               `json:"required,omitempty"`
	Fields      map[string]*FieldEntry `json:"fields,omitempty"`
}

// FieldEntry describes a single property of a schema.
type FieldEntry struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []any    `json:"enum,omitempty"`
	Format      string   `json:"format,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Ref         string   `json:"$ref,omitempty"`
	Const       any      `json:"const,omitempty"`
	Required    bool     `json:"required,omitempty"`
	MinLength   *float64 `json:"minLength,omitempty"`
	MaxLength   *float64 `json:"maxLength,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
}

// LoadBundle reads every *.schema.json file in dir and returns a bundle
// keyed by filename stem (so "hypothesis.schema.json" maps to
// "hypothesis"). The function is tolerant: a malformed schema file is
// skipped with a warning recorded in the returned []error, but other
// schemas still load.
func LoadBundle(dir string) (*Bundle, []error, error) {
	if dir == "" {
		return &Bundle{Schemas: map[string]*SchemaEntry{}}, nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read schema dir: %w", err)
	}
	b := &Bundle{Schemas: map[string]*SchemaEntry{}}
	var warnings []error
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".schema.json") {
			continue
		}
		stem := strings.TrimSuffix(name, ".schema.json")
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", name, err))
			continue
		}
		entry, err := parseSchema(raw)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", name, err))
			continue
		}
		b.Schemas[stem] = entry
	}
	return b, warnings, nil
}

// rawSchema mirrors the subset of JSON Schema draft 2020-12 that the
// bundle projection uses. Anything outside this shape is ignored.
type rawSchema struct {
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Required    []string              `json:"required"`
	Properties  map[string]*rawSchema `json:"properties"`
	Type        any                   `json:"type"`
	Enum        []any                 `json:"enum"`
	Format      string                `json:"format"`
	Pattern     string                `json:"pattern"`
	Ref         string                `json:"$ref"`
	Const       any                   `json:"const"`
	MinLength   *float64              `json:"minLength"`
	MaxLength   *float64              `json:"maxLength"`
	Minimum     *float64              `json:"minimum"`
	Maximum     *float64              `json:"maximum"`
}

func parseSchema(raw []byte) (*SchemaEntry, error) {
	var s rawSchema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	entry := &SchemaEntry{
		Title:       s.Title,
		Description: s.Description,
		Required:    append([]string(nil), s.Required...),
	}
	required := map[string]bool{}
	for _, r := range s.Required {
		required[r] = true
	}
	if len(s.Properties) > 0 {
		entry.Fields = make(map[string]*FieldEntry, len(s.Properties))
		for name, p := range s.Properties {
			entry.Fields[name] = projectField(p, required[name])
		}
	}
	return entry, nil
}

func projectField(p *rawSchema, required bool) *FieldEntry {
	if p == nil {
		return &FieldEntry{Required: required}
	}
	f := &FieldEntry{
		Description: p.Description,
		Enum:        p.Enum,
		Format:      p.Format,
		Pattern:     p.Pattern,
		Ref:         p.Ref,
		Const:       p.Const,
		Required:    required,
		MinLength:   p.MinLength,
		MaxLength:   p.MaxLength,
		Minimum:     p.Minimum,
		Maximum:     p.Maximum,
	}
	f.Type = typeString(p.Type)
	return f
}

// typeString flattens the JSON Schema "type" keyword, which may be a
// single string ("integer") or an array ("[integer,null]" meaning
// nullable), into a display-friendly string.
func typeString(t any) string {
	switch v := t.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		sort.Strings(parts)
		return strings.Join(parts, " | ")
	}
	return ""
}

// Marshal returns the compact JSON encoding of the bundle suitable for
// embedding in an HTML <script type="application/json"> tag.
func (b *Bundle) Marshal() ([]byte, error) {
	if b == nil {
		return []byte(`{"schemas":{}}`), nil
	}
	return json.Marshal(b)
}
