package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tmc/md2html"
)

func main() {
	flags := md2html.NewFlagSet("md2html")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		os.Exit(2)
	}

	// Interrupting the command stops it. Run itself installs no signal
	// handler, because a library that did would change the behavior of
	// whatever program linked it in.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := md2html.ConfigFromFlags(flags)
	if err := md2html.Run(ctx, cfg, slog.Default(), os.Stdout, flags.Args()); err != nil {
		log.Fatal(err)
	}
}
