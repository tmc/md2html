package md2html

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/tmc/md2html/internal/scripttest"
)

func TestMain(m *testing.M) {
	scripttest.TestMain(m, func() {
		flags := NewFlagSet("md2html")
		if err := flags.Parse(os.Args[1:]); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		cfg := ConfigFromFlags(flags)
		// Stand in for cmd/md2html, which owns signal handling: Run
		// installs none, so script fixtures that interrupt the server
		// would otherwise see it killed instead of shut down.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := Run(ctx, cfg, slog.Default(), os.Stdout, flags.Args()); err != nil {
			// Report the error the way cmd/md2html does, so script
			// tests can assert on startup failures.
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	})
}

// TestRunChdir checks that -C resolves relative paths against the named
// directory, and that it does so without changing the process working
// directory: Run is a library call, and a caller that keeps its own
// directory must be able to make two of them.
func TestRunChdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.md"), []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cfg := Config{Chdir: dir, Source: "page.md"}
	if err := Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), &out, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi") {
		t.Errorf("output missing rendered heading:\n%s", out.String())
	}

	after, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if after != wd {
		t.Errorf("working directory = %q after Run, want %q", after, wd)
	}
}

// TestRunChdirIndependent checks that two configurations naming
// different directories do not interfere, which is what not changing the
// process working directory buys.
func TestRunChdirIndependent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dirs := make([]string, 2)
	for i, want := range []string{"one", "two"} {
		dirs[i] = t.TempDir()
		if err := os.WriteFile(filepath.Join(dirs[i], "page.md"), []byte("# "+want+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range []string{"one", "two"} {
		var out bytes.Buffer
		cfg := Config{Chdir: dirs[i], Source: "page.md"}
		if err := Run(context.Background(), cfg, logger, &out, nil); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("run %d rendered %q, want a heading %q", i, out.String(), want)
		}
	}
}

// TestRunChdirNotADirectory checks that -C is validated even when every
// other path is absolute and nothing would otherwise resolve against it.
func TestRunChdirNotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "page.md")
	if err := os.WriteFile(file, []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var out bytes.Buffer
	err := Run(context.Background(), Config{Chdir: file, Source: file}, logger, &out, nil)
	if err == nil {
		t.Fatal("Run succeeded with -C naming a file")
	}
	if !strings.Contains(err.Error(), "chdir") {
		t.Errorf("error = %v, want it to mention chdir", err)
	}

	out.Reset()
	err = Run(context.Background(), Config{Chdir: filepath.Join(dir, "missing"), Source: file}, logger, &out, nil)
	if err == nil {
		t.Fatal("Run succeeded with -C naming a missing directory")
	}
}

func TestRunNilLogger(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.md")
	if err := os.WriteFile(path, []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Run(context.Background(), Config{Source: path}, nil, &out, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Hi") {
		t.Errorf("output missing rendered heading:\n%s", out.String())
	}
}

func TestGenerateStaticHTMLReportsPageErrors(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "page.md"), []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(output, "nested"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := generateStaticHTML(context.Background(), Config{
		Source:  source,
		HTML:    output,
		HTMLExt: "html",
		Drafts:  true,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("generateStaticHTML succeeded with an unwritable page path")
	}
	if !strings.Contains(err.Error(), "process nested/page.md") {
		t.Fatalf("error = %v, want page path", err)
	}
}
