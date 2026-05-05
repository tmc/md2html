package md2html

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed static/js/minisearch.min.js static/js/search.js
var searchAssets embed.FS

// searchAssetFiles lists the embedded client-side search assets that ship
// with the binary. They are written to the output tree (static mode) or
// served from the embedded FS (server mode) so a generated site needs no
// network access at runtime.
var searchAssetFiles = []string{
	"static/js/minisearch.min.js",
	"static/js/search.js",
}

// readSearchAsset returns the bytes of an embedded asset by its embed path.
func readSearchAsset(name string) ([]byte, error) {
	return searchAssets.ReadFile(name)
}

// writeSearchAssets copies the embedded search assets into outputDir/js/.
// It mirrors the layout that templates/search.html references.
func writeSearchAssets(outputDir string) error {
	jsDir := filepath.Join(outputDir, "js")
	if err := os.MkdirAll(jsDir, 0755); err != nil {
		return fmt.Errorf("create js dir: %w", err)
	}
	for _, name := range searchAssetFiles {
		body, err := fs.ReadFile(searchAssets, name)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", name, err)
		}
		out := filepath.Join(jsDir, filepath.Base(name))
		if err := os.WriteFile(out, body, 0644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
	}
	return nil
}
