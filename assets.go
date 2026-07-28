package md2html

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed static/js/minisearch.min.js static/js/search.js static/js/jsonspec.js
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

func writeJSONSpecAsset(outputDir string) error {
	body, err := fs.ReadFile(searchAssets, "static/js/jsonspec.js")
	if err != nil {
		return fmt.Errorf("read embedded jsonspec.js: %w", err)
	}
	jsDir := filepath.Join(outputDir, "js")
	if err := os.MkdirAll(jsDir, 0755); err != nil {
		return fmt.Errorf("create js dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(jsDir, "jsonspec.js"), body, 0644); err != nil {
		return fmt.Errorf("write jsonspec.js: %w", err)
	}
	return nil
}

func writeFingerprintedSearchAssets(outputDir string, searchIndex []byte) (map[string]string, error) {
	assets := map[string][]byte{
		"search-index.js": searchIndex,
	}
	for _, name := range searchAssetFiles {
		body, err := fs.ReadFile(searchAssets, name)
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", name, err)
		}
		assets[path.Join("js", filepath.Base(name))] = body
	}

	out := make(map[string]string)
	for name, body := range assets {
		hashed := fingerprintName(name, body)
		full := filepath.Join(outputDir, filepath.FromSlash(hashed))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return nil, fmt.Errorf("create asset dir: %w", err)
		}
		if err := os.WriteFile(full, body, 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", full, err)
		}
		out[name] = hashed
	}
	return out, nil
}

func fingerprintName(name string, body []byte) string {
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])[:8]
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return base + "." + hash + ext
}
