// Package iconsets provides the icon libraries embedded in md2html.
package iconsets

//go:generate go run ./gen

import (
	"compress/gzip"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

//go:embed fontawesome/data.txt lucide/data.txt tabler/data.txt
var data embed.FS

// A Library is one pinned icon library.
type Library struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Source      string            `json:"source"`
	License     string            `json:"license"`
	Attribution string            `json:"attribution"`
	Icons       map[string]string `json:"icons"`
}

// Load returns the named embedded library.
func Load(name string) (Library, error) {
	encoded, err := data.ReadFile(name + "/data.txt")
	if err != nil {
		return Library{}, fmt.Errorf("unknown icon library %q", name)
	}
	b64 := base64.NewDecoder(base64.StdEncoding, strings.NewReader(string(encoded)))
	zr, err := gzip.NewReader(b64)
	if err != nil {
		return Library{}, fmt.Errorf("open %s icon data: %w", name, err)
	}
	defer zr.Close()
	var lib Library
	if err := json.NewDecoder(zr).Decode(&lib); err != nil {
		return Library{}, fmt.Errorf("decode %s icon data: %w", name, err)
	}
	if _, err := io.Copy(io.Discard, zr); err != nil {
		return Library{}, fmt.Errorf("read %s icon data: %w", name, err)
	}
	return lib, nil
}

// Names returns the supported library names.
func Names() []string {
	return []string{"fontawesome", "lucide", "tabler"}
}
