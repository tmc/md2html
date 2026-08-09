package md2html

import (
	"flag"
	"testing"
)

func TestConfigFromForeignFlagSet(t *testing.T) {
	fs := flag.NewFlagSet("foreign", flag.ContinueOnError)
	fs.String("title", "Foreign title", "")

	cfg := ConfigFromFlags(fs)
	if cfg.Title != "Foreign title" {
		t.Fatalf("Title = %q, want %q", cfg.Title, "Foreign title")
	}
	if cfg.HTTP != "" {
		t.Fatalf("HTTP = %q, want empty", cfg.HTTP)
	}
}

func TestChdirFlag(t *testing.T) {
	fs := NewFlagSet("md2html")
	if err := fs.Parse([]string{"-C", "/somewhere"}); err != nil {
		t.Fatal(err)
	}
	if cfg := ConfigFromFlags(fs); cfg.Chdir != "/somewhere" {
		t.Fatalf("Chdir = %q, want /somewhere", cfg.Chdir)
	}
}
