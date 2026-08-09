package md2html

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/md2html/internal/scripttest"
)

func TestMain(m *testing.M) {
	scripttest.TestMain(m, func() {
		flags := NewFlagSet("md2html")
		flags.Parse(os.Args[1:])
		cfg := ConfigFromFlags(flags)
		if err := Run(context.Background(), cfg, slog.Default(), os.Stdout, flags.Args()); err != nil {
			// Report the error the way cmd/md2html does, so script
			// tests can assert on startup failures.
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	})
}

// TestRunChdir checks that -C resolves relative paths against the named
// directory rather than the process working directory.
func TestRunChdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.md"), []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Run chdirs the process; put it back for later tests.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	var out bytes.Buffer
	cfg := Config{Chdir: dir, Source: "page.md"}
	if err := Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), &out, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi") {
		t.Errorf("output missing rendered heading:\n%s", out.String())
	}
}
