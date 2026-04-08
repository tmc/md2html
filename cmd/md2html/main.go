package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/tmc/md2html"
)

func main() {
	flags := md2html.NewFlagSet("md2html")
	flags.Parse(os.Args[1:])

	cfg := md2html.ConfigFromFlags(flags)
	if err := md2html.Run(context.Background(), cfg, slog.Default(), os.Stdout, flags.Args()); err != nil {
		log.Fatal(err)
	}
}
