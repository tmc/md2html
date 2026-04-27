package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/md2html/internal/mdvet"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("mdvet", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: mdvet [flags] path...\n\n")
		fs.PrintDefaults()
	}
	checksFlag := fs.String("c", "", "comma-separated list of checks to run (default: all)")
	listFlag := fs.Bool("list", false, "print the available checks and exit")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	all := mdvet.AllChecks()
	if *listFlag {
		for _, c := range all {
			fmt.Fprintln(stdout, c.Name())
		}
		return 0
	}

	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}

	var names []string
	if *checksFlag != "" {
		for n := range strings.SplitSeq(*checksFlag, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				names = append(names, n)
			}
		}
	}
	checks, err := mdvet.SelectChecks(all, names)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	diags, err := mdvet.Run(fs.Args(), checks)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	for _, d := range diags {
		fmt.Fprintln(stdout, d)
	}
	if len(diags) > 0 {
		return 1
	}
	return 0
}
