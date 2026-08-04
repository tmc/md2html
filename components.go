package md2html

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tmc/md2html/internal/markdown/components"
)

// prepareComponents resolves [Config.Components] into a component
// registry. A directory that cannot be read, or a component template
// that cannot be parsed, is a configuration error: rendering with a
// half-loaded registry would report every component in the site as
// unknown.
func prepareComponents(cfg Config) (Config, error) {
	if cfg.componentRegistry != nil || strings.TrimSpace(cfg.Components) == "" {
		return cfg, nil
	}
	dir, err := filepath.Abs(cfg.Components)
	if err != nil {
		return cfg, fmt.Errorf("resolve components directory: %w", err)
	}
	reg, err := components.LoadRegistry(dir)
	if err != nil {
		return cfg, fmt.Errorf("load components: %w", err)
	}
	cfg.Components = dir
	cfg.componentRegistry = reg
	return cfg, nil
}

// componentRegistry returns the registry to render with, falling back to
// the built-in components when no directory was configured.
func (cfg Config) componentsRegistry() components.Registry {
	if cfg.componentRegistry != nil {
		return cfg.componentRegistry
	}
	return components.DefaultRegistry
}
