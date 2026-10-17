package docversions

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// CopyArchive copies the archive at src, a directory or an http or
// https URL of a .tar.gz, to the directory dst. Every .html file has
// inject(rel) inserted at the top of its body, where rel is the file's
// slash-separated path within the archive. Nothing else is changed.
func CopyArchive(ctx context.Context, src, dst string, inject func(rel string) []byte) error {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return fetchArchive(ctx, src, dst, inject)
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return writeArchived(out, filepath.ToSlash(rel), data, inject)
	})
}

func fetchArchive(ctx context.Context, url, dst string, inject func(rel string) []byte) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching archive %s: %s", url, resp.Status)
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("reading archive %s: %w", url, err)
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading archive %s: %w", url, err)
		}
		rel := filepath.ToSlash(filepath.Clean(h.Name))
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("reading archive %s: %s is outside the archive", url, h.Name)
		}
		out := filepath.Join(dst, filepath.FromSlash(rel))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			if err != nil {
				return err
			}
			if err := writeArchived(out, rel, data, inject); err != nil {
				return err
			}
		}
	}
}

func writeArchived(out, rel string, data []byte, inject func(rel string) []byte) error {
	if inject != nil && strings.HasSuffix(rel, ".html") {
		data = injectBody(data, inject(rel))
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, data, 0o644)
}

// injectBody inserts snippet just after the opening body tag of page,
// or at its start if it has none.
func injectBody(page, snippet []byte) []byte {
	i := bytes.Index(bytes.ToLower(page), []byte("<body"))
	if i < 0 {
		return append(snippet[:len(snippet):len(snippet)], page...)
	}
	end := bytes.IndexByte(page[i:], '>')
	if end < 0 {
		return page
	}
	at := i + end + 1
	out := make([]byte, 0, len(page)+len(snippet))
	out = append(out, page[:at]...)
	out = append(out, snippet...)
	return append(out, page[at:]...)
}
