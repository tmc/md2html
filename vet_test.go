package md2html

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"),
		[]byte("[bad](missing.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := Config{Source: filepath.Join(dir, "x.md"), Vet: true}
	runVet(cfg, logger)

	out := buf.String()
	if !strings.Contains(out, "missing.md") {
		t.Errorf("expected diagnostic in vet output, got %q", out)
	}
	if !strings.Contains(out, `check=assets`) {
		t.Errorf("expected check=assets in vet output, got %q", out)
	}
}

func TestRunVet_SkipStdin(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	runVet(Config{Source: "-", Vet: true}, logger)
	if buf.Len() != 0 {
		t.Errorf("expected no output for stdin source, got %q", buf.String())
	}
}

func TestRunVet_UnknownCheck(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"),
		[]byte("[bad](missing.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	cfg := Config{Source: filepath.Join(dir, "x.md"), Vet: true, VetChecks: "links,nope"}
	runVet(cfg, logger)

	out := buf.String()
	if !strings.Contains(out, "unknown checks") {
		t.Errorf("expected warning about unknown checks, got %q", out)
	}
	if !strings.Contains(out, "missing.md") {
		t.Errorf("expected the known check to still run, got %q", out)
	}
}

func TestSelectVetChecks(t *testing.T) {
	checks, err := selectVetChecks("")
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) == 0 {
		t.Fatal("empty spec should return all checks")
	}

	checks, err = selectVetChecks("links")
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Name() != "links" {
		t.Errorf("expected [links], got %v", checks)
	}

	checks, err = selectVetChecks("links,nope")
	if err == nil {
		t.Fatal("expected error for unknown check")
	}
	if len(checks) != 1 {
		t.Errorf("expected known check still returned, got %v", checks)
	}
}
