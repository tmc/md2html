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

// prepareJSONSpec prepares a site with only JSONSpec loaded from cfg.
func prepareJSONSpec(cfg Config, logger *slog.Logger) (*preparedSite, error) {
	s := &preparedSite{config: cfg}
	if err := s.prepareJSONSpec(logger); err != nil {
		return nil, err
	}
	return s, nil
}
