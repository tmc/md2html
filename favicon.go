package md2html

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// faviconTypes are the image formats browsers take a site icon in.
var faviconTypes = map[string]string{
	".ico":  "image/x-icon",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".gif":  "image/gif",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// prepareFavicon finds the site icon: the file -favicon names, or else
// the one named by the docs.json at or above sourceDir. A file named
// on the command line must exist. One docs.json names is skipped with
// a warning when it does not, as that docs.json may be configuration
// for a hosted site whose files are elsewhere.
func (s *preparedSite) prepareFavicon(sourceDir string, logger *slog.Logger) error {
	file := s.config.Favicon
	fromFlag := file != ""
	if !fromFlag {
		doc, siteDir, ok := readDocsJSON(sourceDir)
		if !ok || doc.Favicon == "" || strings.Contains(string(doc.Favicon), "://") {
			return nil
		}
		file = filepath.Join(siteDir, filepath.FromSlash(strings.TrimLeft(string(doc.Favicon), "/")))
	}
	ext := strings.ToLower(filepath.Ext(file))
	var err error
	if _, ok := faviconTypes[ext]; !ok {
		err = fmt.Errorf("%s is not an image a browser shows as a site icon", file)
	} else if info, statErr := os.Stat(file); statErr != nil {
		err = statErr
	} else if !info.Mode().IsRegular() {
		err = fmt.Errorf("%s is not a regular file", file)
	}
	if err != nil {
		if fromFlag {
			return fmt.Errorf("favicon: %w", err)
		}
		logger.Warn("skipping the favicon docs.json names", "error", err)
		return nil
	}
	s.favicon = file
	s.faviconName = "favicon" + ext
	return nil
}

// copyFavicon writes the site icon into the root of a static build.
func (s *preparedSite) copyFavicon(outputDir string) error {
	if s.favicon == "" {
		return nil
	}
	src, err := os.Open(s.favicon)
	if err != nil {
		return fmt.Errorf("favicon: %w", err)
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(outputDir, s.faviconName))
	if err != nil {
		return fmt.Errorf("favicon: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("favicon: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("favicon: %w", err)
	}
	return nil
}

// serveFavicon serves the site icon.
func (s *preparedSite) serveFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", faviconTypes[filepath.Ext(s.faviconName)])
	http.ServeFile(w, r, s.favicon)
}
