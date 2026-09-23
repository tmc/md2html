package md2html

import (
	"context"
	"log/slog"
)

// generateStaticHTML prepares a site from cfg and writes its static build.
func generateStaticHTML(ctx context.Context, cfg Config, logger *slog.Logger) error {
	site, err := prepareSite(cfg, logger)
	if err != nil {
		return err
	}
	return site.generateStaticHTML(ctx, logger)
}
