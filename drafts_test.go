package md2html

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateStaticHTMLDrafts(t *testing.T) {
	tests := []struct {
		name      string
		drafts    bool
		wantDraft bool
	}{
		{name: "default skips drafts", drafts: false, wantDraft: false},
		{name: "drafts flag renders drafts", drafts: true, wantDraft: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			pages := map[string]string{
				"published.md": "---\ntitle: Published\ndraft: false\n---\n# Published\n",
				"pending.md":   "---\ntitle: Pending\ndraft: true\n---\n# Pending\n",
			}
			for name, content := range pages {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			out := filepath.Join(root, "out")
			cfg := Config{Source: root, HTML: out, HTMLExt: "html", Drafts: tt.drafts}
			if err := generateStaticHTML(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
				t.Fatalf("generateStaticHTML() error = %v", err)
			}

			if _, err := os.Stat(filepath.Join(out, "published.html")); err != nil {
				t.Fatalf("published page missing: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(out, "pending.html"))
			if tt.wantDraft {
				if err != nil {
					t.Fatalf("draft page missing with Drafts enabled: %v", err)
				}
				if !strings.Contains(string(data), "Pending") {
					t.Fatalf("draft page rendered without content:\n%s", data)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("draft page rendered without Drafts; err = %v", err)
			}
		})
	}
}
