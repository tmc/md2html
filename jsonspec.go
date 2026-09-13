package md2html

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/md2html/internal/markdown/jsonspec"
)

type jsonSpecFile struct {
	Prefixes   []string `json:"prefixes"`
	BadgeURL   string   `json:"badge_url"`
	BadgeLabel string   `json:"badge_label"`
}

func (s *preparedSite) prepareJSONSpec(logger *slog.Logger) error {
	if s.jsonSpecReady || strings.TrimSpace(s.config.JSONSpec) == "" {
		return nil
	}
	dir, err := filepath.Abs(s.config.JSONSpec)
	if err != nil {
		return fmt.Errorf("resolve jsonspec directory: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "jsonspec.json"))
	if err != nil {
		return fmt.Errorf("read jsonspec config: %w", err)
	}
	var file jsonSpecFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("parse jsonspec config: %w", err)
	}
	for i := range file.Prefixes {
		file.Prefixes[i] = strings.TrimSpace(file.Prefixes[i])
	}
	s.jsonSpecConfig = jsonspec.Config{
		DiscriminatorPrefixes: file.Prefixes,
		BadgeURLTemplate:      file.BadgeURL,
		BadgeLabelTemplate:    file.BadgeLabel,
	}
	bundle, warnings, err := jsonspec.LoadBundle(dir)
	if err != nil {
		return fmt.Errorf("load jsonspec bundle: %w", err)
	}
	if logger != nil {
		for _, warning := range warnings {
			logger.Warn("JSON schema load warning", "directory", dir, "error", warning)
		}
	}
	raw, err = bundle.Marshal()
	if err != nil {
		return fmt.Errorf("marshal jsonspec bundle: %w", err)
	}
	s.config.JSONSpec = dir
	s.jsonSpecBundle = template.JS(raw)
	s.jsonSpecReady = true
	return nil
}

// prepareJSONSpec prepares a site with only JSONSpec loaded from cfg.
func prepareJSONSpec(cfg Config, logger *slog.Logger) (*preparedSite, error) {
	s := &preparedSite{config: cfg}
	if err := s.prepareJSONSpec(logger); err != nil {
		return nil, err
	}
	return s, nil
}
