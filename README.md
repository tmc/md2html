# md2html

`md2html` renders Markdown as HTML: a single file to standard output, a
live-reloading HTTP server, or a static HTML site. The repository also provides
`mdvet`, a vet-style checker for Markdown trees, and package
`github.com/tmc/md2html` for in-process rendering and navigation parsing.

## Install

```sh
go install github.com/tmc/md2html/cmd/md2html@latest
go install github.com/tmc/md2html/cmd/mdvet@latest
```

## Usage

Render a single Markdown file to standard output:

```sh
md2html README.md
```

Serve the current directory over HTTP with live reload (use `-watch=false` to
disable file watching, or `-base /docs` to preview under a URL prefix):

```sh
md2html -http :8080
```

Build a static HTML site with docs navigation (add `-search` for client-side
search, `-llms` for `llms.txt` output, or `-versions` to publish git tags
alongside the working tree):

```sh
md2html -nav -html _site .
```

Run checks on a Markdown tree (use `-list` to see the available checks):

```sh
mdvet .
mdvet -list
```

## Features

- **Markdown rendering**: GitHub Flavored Markdown with syntax highlighting,
  footnotes, GitHub alerts, fenced admonitions, tabs, audio/video embeds,
  Mermaid diagrams, and MathJax. Raw HTML is dropped by default
  (`-allow-unsafe` passes it through).
- **Navigation and icons**: Builds sidebar navigation from `SUMMARY.md`, a
  Mintlify `docs.json`, or the directory tree (`-nav`). Embeds Font Awesome
  Free, Lucide, and Tabler Outline icon sets; override with `-icons dir` or
  disable with `-no-icons`.
- **Components and code includes**: Built-in MDX-style layout components
  (`Card`, `CardGroup`, `Steps`, `Accordion`, `Frame`), custom template-backed
  components (`-components dir`), and `<Code src="..." />` file and region
  includes (`-include-root dir`).
- **Static output and metadata**: Client-side search (`-search`), `llms.txt`
  and `llms-full.txt` (`-llms`), Open Graph and Twitter card metadata
  (`-site-url`, `-og-image`), directory-style URLs (`-html-layout dir`),
  allowlisted static assets, and `.md2htmlignore` / `.mintignore` exclusions.
- **Versioned documentation**: Builds one output directory per matching git tag
  or branch alongside the working tree (`-versions`, `-version-pattern`,
  `versions.yaml`), with rename tracking, build caching (`-version-cache`), and
  aliases (`-version-alias`).
- **Custom templates**: Override any embedded `html/template` definition with
  `-templates dir`; see [examples/README.md](examples/README.md).

## Library

Import `github.com/tmc/md2html` to render Markdown fragments, parse navigation,
or embed the server and static builder:

```go
frag, err := md2html.RenderFragment("# Hello\n", "", md2html.FragmentOptions{})
if err != nil {
	log.Fatal(err)
}
fmt.Println(frag.HTML)
```

See [pkg.go.dev/github.com/tmc/md2html](https://pkg.go.dev/github.com/tmc/md2html)
for the full API (`RenderFragment`, `NewLoader`, `ParseSummary`, and `Run`).

## Documentation

Full command flags, configuration formats (`docs.json`, `versions.yaml`,
`components.json`, `jsonspec.json`), and Markdown dialect details are in the
package docs:

```sh
go doc github.com/tmc/md2html/cmd/md2html
go doc github.com/tmc/md2html/cmd/mdvet
go doc github.com/tmc/md2html
```

Template override examples are in [examples/README.md](examples/README.md).
