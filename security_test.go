package md2html

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderReadFileRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir, ".html")

	if err := os.WriteFile(filepath.Join(dir, "ok.md"), []byte("# ok\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loader.Load("ok.md"); err != nil {
		t.Fatalf("Load(ok.md) error = %v", err)
	}

	if _, err := loader.Load("../secret.md"); err == nil {
		t.Fatal("Load(../secret.md) succeeded, want error")
	}
}

func TestHandleIndexRejectsTraversalPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("# root\n"), 0644); err != nil {
		t.Fatal(err)
	}
	secretDir := filepath.Join(dir, "..")
	if err := os.WriteFile(filepath.Join(secretDir, "secret.md"), []byte("# secret\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := newServer(Config{Source: dir}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, target := range []string{"/../secret", "/?file=../secret.md"} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			s.handleIndex(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleIndexServesStaticAssets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clip.mp4"), []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}

	s := newServer(Config{Source: dir}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/clip.mp4", nil)
	rec := httptest.NewRecorder()
	s.handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "video" {
		t.Fatalf("body = %q, want %q", body, "video")
	}
}
