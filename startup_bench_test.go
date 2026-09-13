package md2html

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkNewServerHomeTmp(b *testing.B) {
	dir := b.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "guide.md"), []byte("# Guide\n"), 0644); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0644); err != nil {
		b.Fatal(err)
	}

	oldwd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		b.Fatal(err)
	}
	defer os.Chdir(oldwd)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := Config{HTTP: ":0"}
	site, err := prepareSite(cfg, logger)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		s := newServer(context.Background(), site, logger)
		if s == nil {
			b.Fatal("newServer returned nil")
		}
	}
}
