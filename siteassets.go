package md2html

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// assetExtensions are the file types copied from the source tree into
// static output.
//
// The set is an allowlist rather than "everything that is not Markdown"
// because static output gets published: a docs tree usually sits inside
// a repository that also holds source, notes, and configuration, and
// none of that should reach a web host because it happened to be next
// to a page. What a page can reference and a browser can render is the
// line.
var assetExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".webp": true, ".avif": true, ".bmp": true, ".ico": true,

	// Markdown image syntax pointing at these renders as media controls.
	".mp4": true, ".webm": true, ".mov": true,
	".mp3": true, ".wav": true, ".ogg": true, ".m4a": true,

	".woff": true, ".woff2": true, ".ttf": true, ".otf": true,

	// Files a page or a host asks for by name: stylesheets and scripts
	// a template pulls in, robots.txt, a web app manifest, a sitemap.
	".css": true, ".js": true, ".pdf": true, ".txt": true,
	".xml": true, ".webmanifest": true,
}

// copySourceAssets copies the non-Markdown files a published site needs
// from sourceDir to outputDir, keeping their paths, and reports how many
// it wrote. Rendering only writes HTML, so without this a page's images
// resolve when served from the source tree and 404 once deployed.
//
// Exclusions follow the rest of the tree walk: the ignore file, dot
// files and dot directories, and the output directory when it sits
// inside the source. A file that cannot be read is reported and skipped
// rather than failing the build.
func copySourceAssets(sourceDir, outputDir string, logger *slog.Logger) (int, error) {
	source, err := filepath.Abs(sourceDir)
	if err != nil {
		return 0, err
	}
	output, err := filepath.Abs(outputDir)
	if err != nil {
		return 0, err
	}
	ignore, err := loadIgnoreSet(source)
	if err != nil {
		logger.Warn("Skipping the ignore file", "error", err)
	}

	var copied int
	err = filepath.WalkDir(source, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			logger.Warn("Skipping path", "path", p, "error", err)
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p == output {
				return filepath.SkipDir
			}
			if p != source && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if p != source && ignore.excludes(p, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !assetExtensions[strings.ToLower(filepath.Ext(name))] {
			return nil
		}
		if ignore.excludes(p, false) {
			return nil
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return nil
		}
		if err := copyFile(p, filepath.Join(output, rel)); err != nil {
			logger.Warn("Skipping asset", "path", rel, "error", err)
			return nil
		}
		copied++
		return nil
	})
	return copied, err
}

// copyFile copies the contents of src to dest, creating the directories
// dest needs. It streams rather than reading the file in, because a page
// may point at a video.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
