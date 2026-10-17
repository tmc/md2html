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

// writeFingerprintedSearchAssets writes the search index and the search
// scripts under outputDir, each named for its content, and returns the
// URL of each relative to outputDir. With shared, the scripts, which
// are the same in every version of a versioned site, are written to the
// shared directory instead; the index is the site's own.
func writeFingerprintedSearchAssets(outputDir string, searchIndex []byte, shared *sharedAssets) (map[string]string, error) {
	out := make(map[string]string)
	write := func(dir, name string, body []byte) (string, error) {
		hashed := fingerprintName(name, body)
		full := filepath.Join(dir, filepath.FromSlash(hashed))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return "", fmt.Errorf("create asset dir: %w", err)
		}
		if err := os.WriteFile(full, body, 0644); err != nil {
			return "", fmt.Errorf("write %s: %w", full, err)
		}
		return hashed, nil
	}
	hashed, err := write(outputDir, "search-index.js", searchIndex)
	if err != nil {
		return nil, err
	}
	out["search-index.js"] = hashed
	for _, file := range searchAssetFiles {
		body, err := fs.ReadFile(searchAssets, file)
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", file, err)
		}
		name := path.Join("js", filepath.Base(file))
		if shared == nil {
			if out[name], err = write(outputDir, name, body); err != nil {
				return nil, err
			}
			continue
		}
		hashed, err := write(shared.dir, name, body)
		if err != nil {
			return nil, err
		}
		shared.files = append(shared.files, hashed)
		out[name] = shared.prefix + hashed
	}
	return out, nil
}

// sharedAssets is where a version of a versioned static site writes the
// content-named files it shares with the other versions, so that the
// site holds one copy of each.
type sharedAssets struct {
	dir    string   // the shared directory
	prefix string   // URL of dir relative to the version's root
	files  []string // files written, relative to dir
}

func fingerprintName(name string, body []byte) string {
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])[:8]
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return base + "." + hash + ext
}
