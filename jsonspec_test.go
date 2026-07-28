package md2html

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareJSONSpec(t *testing.T) {
	dir := t.TempDir()
	config := `{"prefixes":["ascf/"],"badge_url":"schemas.html#%s","badge_label":"%s schema"}`
	if err := os.WriteFile(filepath.Join(dir, "jsonspec.json"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	schema := `{"properties":{"id":{"type":"string"}}}`
	if err := os.WriteFile(filepath.Join(dir, "hypothesis.schema.json"), []byte(schema), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := prepareJSONSpec(Config{JSONSpec: dir}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.jsonSpecConfig.DiscriminatorPrefixes) != 1 || cfg.jsonSpecConfig.DiscriminatorPrefixes[0] != "ascf/" {
		t.Fatalf("prefixes = %v, want [ascf/]", cfg.jsonSpecConfig.DiscriminatorPrefixes)
	}
	if cfg.jsonSpecBundle == "" {
		t.Fatal("jsonSpecBundle is empty")
	}
}

func TestJSONSpecFlagSurface(t *testing.T) {
	fs := NewFlagSet("test")
	if fs.Lookup("jsonspec") == nil {
		t.Fatal("-jsonspec is not registered")
	}
	for _, name := range []string{"jsonspec-prefixes", "jsonspec-badge-url", "jsonspec-badge-label", "jsonspec-schemas"} {
		if fs.Lookup(name) != nil {
			t.Fatalf("legacy flag -%s is still registered", name)
		}
	}
}
