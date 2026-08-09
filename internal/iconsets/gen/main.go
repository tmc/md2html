// Command gen downloads pinned icon releases and generates embedded data.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type source struct {
	name, version, url, sum, attribution string
	keep                                 func(string) (string, bool)
}

var sources = []source{
	{"fontawesome", "7.2.0", "https://codeload.github.com/FortAwesome/Font-Awesome/tar.gz/refs/tags/7.2.0", "8f433b74d3d4bbba2a6374a0a0ddf53be8e127809341e4c2578719229b43903a", "Font Awesome Free 7.2.0 by Fonticons, Inc. (https://fontawesome.com), licensed under CC BY 4.0 (https://creativecommons.org/licenses/by/4.0/)", fontAwesome},
	{"lucide", "1.27.0", "https://codeload.github.com/lucide-icons/lucide/tar.gz/refs/tags/1.27.0", "99806eb8f855c167600854c29340a7a906207256cf2b3070be1152f65725a191", "Lucide 1.27.0 (https://lucide.dev), licensed under ISC; derived Feather icons are licensed under MIT", flat("/icons/")},
	{"tabler", "3.46.0", "https://codeload.github.com/tabler/tabler-icons/tar.gz/refs/tags/v3.46.0", "ebb50a676311f16390c0a13cc5a102e010a3d082b3dc315bc0273ded0bd1cbf8", "Tabler Icons 3.46.0 (https://tabler.io/icons), licensed under MIT", flat("/icons/outline/")},
}

type library struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Source      string            `json:"source"`
	License     string            `json:"license"`
	Attribution string            `json:"attribution"`
	Icons       map[string]string `json:"icons"`
}

type provenance struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	SHA256  string `json:"sha256"`
	Icons   int    `json:"icons"`
}

func main() {
	for _, src := range sources {
		lib, err := fetch(src)
		if err != nil {
			fatal(err)
		}
		encoded, err := encode(lib)
		if err != nil {
			fatal(err)
		}
		if err := os.MkdirAll(src.name, 0o755); err != nil {
			fatal(err)
		}
		write(src.name+"/data.txt", []byte(encoded+"\n"))
		write(src.name+"/LICENSE.txt", []byte(lib.License))
		meta, _ := json.MarshalIndent(provenance{src.name, src.version, src.url, src.sum, len(lib.Icons)}, "", "  ")
		write(src.name+"/provenance.json", append(meta, '\n'))
	}
}

func fontAwesome(name string) (string, bool) {
	for _, style := range []string{"solid", "regular", "brands"} {
		needle := "/svgs/" + style + "/"
		if strings.Contains(name, needle) && strings.HasSuffix(name, ".svg") {
			return style + "/" + strings.TrimSuffix(path.Base(name), ".svg"), true
		}
	}
	return "", false
}

func flat(dir string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		i := strings.Index(name, dir)
		if i < 0 || !strings.HasSuffix(name, ".svg") || strings.Contains(name[i+len(dir):], "/") {
			return "", false
		}
		return strings.TrimSuffix(path.Base(name), ".svg"), true
	}
}

func fetch(src source) (library, error) {
	client := http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(src.url)
	if err != nil {
		return library{}, fmt.Errorf("fetch %s: %w", src.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return library{}, fmt.Errorf("fetch %s: %s", src.name, resp.Status)
	}
	archive, err := io.ReadAll(resp.Body)
	if err != nil {
		return library{}, err
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != src.sum {
		return library{}, fmt.Errorf("fetch %s: checksum mismatch", src.name)
	}
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return library{}, err
	}
	lib := library{Name: src.name, Version: src.version, Source: src.url, Attribution: src.attribution, Icons: make(map[string]string)}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return library{}, err
		}
		key, keep := src.keep(h.Name)
		license := path.Base(h.Name) == "LICENSE" || path.Base(h.Name) == "LICENSE.txt"
		if !keep && !license {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return library{}, err
		}
		if keep {
			lib.Icons[key] = string(data)
		} else if lib.License == "" {
			lib.License = string(data)
		}
	}
	if len(lib.Icons) == 0 || lib.License == "" {
		return library{}, fmt.Errorf("fetch %s: incomplete archive", src.name)
	}
	return lib, nil
}

func encode(lib library) (string, error) {
	data, err := json.Marshal(lib) // encoding/json sorts map keys.
	if err != nil {
		return "", err
	}
	var compressed bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	zw.Header.ModTime = time.Time{}
	if _, err := zw.Write(data); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(compressed.Bytes()), nil
}

func write(name string, data []byte) {
	if err := os.WriteFile(name, data, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
