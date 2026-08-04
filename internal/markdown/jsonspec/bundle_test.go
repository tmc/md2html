package jsonspec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBundle(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("hypothesis.schema.json", `{
		"title": "Hypothesis",
		"description": "A proposal.",
		"required": ["type", "id"],
		"properties": {
			"type": {"const": "ascf/hypothesis"},
			"id": {"type": "string", "description": "Unique id."},
			"strategy_tag": {"type": "string", "enum": ["local_search", "crossover"]},
			"iteration": {"type": "integer", "minimum": 1},
			"nullable": {"type": ["string", "null"]}
		}
	}`)
	write("broken.schema.json", `{not json`)
	write("ignored.json", `{}`) // wrong suffix

	b, warns, err := LoadBundle(dir)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if len(warns) != 1 {
		t.Errorf("want 1 warning (broken schema), got %d: %v", len(warns), warns)
	}

	h := b.Schemas["hypothesis"]
	if h == nil {
		t.Fatal("hypothesis entry missing")
	}
	if h.Title != "Hypothesis" {
		t.Errorf("title: got %q", h.Title)
	}
	if got, want := len(h.Required), 2; got != want {
		t.Errorf("required count: got %d want %d", got, want)
	}
	if !h.Fields["id"].Required {
		t.Error("id should be marked required")
	}
	if h.Fields["strategy_tag"].Required {
		t.Error("strategy_tag should not be marked required")
	}
	if got := len(h.Fields["strategy_tag"].Enum); got != 2 {
		t.Errorf("strategy_tag enum size: got %d want 2", got)
	}
	if got := h.Fields["nullable"].Type; got != "null | string" {
		t.Errorf("nullable type union: got %q want %q", got, "null | string")
	}
	if _, exists := b.Schemas["ignored"]; exists {
		t.Error("ignored.json should not be loaded (wrong suffix)")
	}
}

func TestLoadBundleEmpty(t *testing.T) {
	b, warns, err := LoadBundle("")
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Error("empty dir should produce no warnings")
	}
	if len(b.Schemas) != 0 {
		t.Error("empty dir should produce empty bundle")
	}
}

func TestBundleMarshal(t *testing.T) {
	b := &Bundle{
		Schemas: map[string]*SchemaEntry{
			"x": {Title: "X", Fields: map[string]*FieldEntry{
				"a": {Type: "string", Required: true},
			}},
		},
	}
	out, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var round Bundle
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if round.Schemas["x"].Fields["a"].Type != "string" {
		t.Errorf("round-trip lost field type: %s", out)
	}
}
