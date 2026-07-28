package md2html

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/md2html/internal/jsonspec"
)

type jsonSpecFile struct {
	Prefixes   []string `json:"prefixes"`
	BadgeURL   string   `json:"badge_url"`
	BadgeLabel string   `json:"badge_label"`
}

func prepareJSONSpec(cfg Config, logger *slog.Logger) (Config, error) {
	if cfg.jsonSpecReady || strings.TrimSpace(cfg.JSONSpec) == "" {
		return cfg, nil
	}
	dir, err := filepath.Abs(cfg.JSONSpec)
	if err != nil {
		return cfg, fmt.Errorf("resolve jsonspec directory: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "jsonspec.json"))
	if err != nil {
		return cfg, fmt.Errorf("read jsonspec config: %w", err)
	}
	var file jsonSpecFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return cfg, fmt.Errorf("parse jsonspec config: %w", err)
	}
	for i := range file.Prefixes {
		file.Prefixes[i] = strings.TrimSpace(file.Prefixes[i])
	}
	cfg.jsonSpecConfig = jsonspec.Config{
		DiscriminatorPrefixes: file.Prefixes,
		BadgeURLTemplate:      file.BadgeURL,
		BadgeLabelTemplate:    file.BadgeLabel,
	}
	bundle, warnings, err := jsonspec.LoadBundle(dir)
	if err != nil {
		return cfg, fmt.Errorf("load jsonspec bundle: %w", err)
	}
	for _, warning := range warnings {
		logger.Warn("JSON schema load warning", "directory", dir, "error", warning)
	}
	raw, err = bundle.Marshal()
	if err != nil {
		return cfg, fmt.Errorf("marshal jsonspec bundle: %w", err)
	}
	cfg.JSONSpec = dir
	cfg.jsonSpecBundle = template.JS(raw)
	cfg.jsonSpecReady = true
	return cfg, nil
}
