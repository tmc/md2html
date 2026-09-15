package md2html

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tmc/md2html/internal/markdown/components"
)

// prepareComponents resolves [Config.Components] into a component
// registry on the prepared site. A directory that cannot be read, or a
// component template that cannot be parsed, is a configuration error:
// rendering with a half-loaded registry would report every component in
// the site as unknown.
func (s *preparedSite) prepareComponents() error {
	if s.componentRegistry != nil || strings.TrimSpace(s.config.Components) == "" {
		return nil
	}
	dir, err := filepath.Abs(s.config.Components)
	if err != nil {
		return fmt.Errorf("resolve components directory: %w", err)
	}
	reg, err := components.LoadRegistry(dir)
	if err != nil {
		return fmt.Errorf("load components: %w", err)
	}
	s.config.Components = dir
	s.componentRegistry = reg
	return nil
}

// componentsRegistry returns the registry to render with, falling back to
// the built-in components when no directory was configured.
func (s *preparedSite) componentsRegistry() components.Registry {
	if s != nil && s.componentRegistry != nil {
		return s.componentRegistry
	}
	return components.DefaultRegistry()
}
