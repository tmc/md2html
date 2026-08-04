package md2html

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
