# md2html

Command md2html renders Markdown as HTML.

It can:

  - render a single Markdown file to stdout
  - serve a Markdown tree over HTTP with live reload
  - generate a static HTML tree

Basic usage:

	md2html README.md

Serve the current tree:

	md2html -http :8080

Disable live-reload file watching:

	md2html -watch=false -http :8080

Generate static output:

	md2html -html _site .

Generate static output with docs navigation:

	md2html -nav -html _site .

`-nav` takes navigation from `SUMMARY.md`, then from a Mintlify `docs.json` covering the tree, then from the shape of the tree itself. A `docs.json` is looked for in the source directory and its parents, since it sits at the root of the published site while the Markdown often lives in a subdirectory; its page paths are resolved against that root, and pages outside the directory being served are skipped. Its `name` becomes the site title unless `-title` says otherwise.

Generate static output with agent-readable summaries:

	md2html -html _site -llms .

Markdown supports GitHub-style alerts such as "> \[!NOTE]" and fenced admonitions such as "!!!note". Image syntax pointing at audio or video files, for example "!\[demo](demo.mp4)", renders as media controls.

Rendering follows GFM, so a single newline is a space and paragraphs reflow to the viewport. Subscript ("H\~2\~O"), superscript ("X^2^"), and definition lists are not supported. Raw HTML is dropped by default; `-allow-unsafe` passes it through, along with attribute syntax and link rewriting inside HTML blocks.

Docs that link to themselves with root-absolute paths, such as `/docs/quickstart`, only resolve when the tree is served under the prefix those links assume. Use `-base` to preview such a subtree:

	cd docs && md2html -http :8080 -base /docs

Pages are then served under `/docs/`, and a request outside the prefix redirects into it. Serving the parent directory instead works whenever the directory name already matches the prefix.

Served pages take their title from the frontmatter `title`, then the first heading, then the file name. A directory URL serves that directory's index file (`index.md` or `README.md`, or the file named by `-index`) and otherwise a listing of the Markdown files beneath it, to `-depth` levels.

Mermaid diagrams and TeX math are rendered in the browser by scripts loaded from a CDN, so those pages need network access on first view. When a script fails to load the page says so rather than leaving the block unrendered.

MDX-style layout components are supported for `Card`, `CardGroup` (also spelled `Columns`), `Steps`/`Step`, `Accordion`/`AccordionGroup` (`Expandable` is the same disclosure), and `Frame`:

	<CardGroup cols={2}>

	<Card title="Install" href="/install">
	Run `go install` to get started.
	</Card>

	</CardGroup>

A tag must stand alone on its line, and its body is ordinary Markdown. This is not MDX: there is no JavaScript, so `import` and `export` are unsupported and brace expressions accept only JSON scalars. Unknown component names, unknown attributes, and missing required attributes are reported as warnings and left unrendered.

Define your own with `-components dir`, where `dir` holds a `components.json` naming each component's attributes and its `html/template` file:

	{
	  "components": {
	    "Note": {
	      "attrs": ["title"],
	      "required": ["title"],
	      "template": "note.html"
	    }
	  }
	}

Each template must reference `{{.Content}}` exactly once, where the Markdown body goes, and reaches attribute values through `{{.Attrs.title}}`. Templates are configuration and may emit any markup; attribute values come from Markdown and are always escaped. A definition may reuse a built-in name to replace it. Pass the same directory to `mdvet -components` so it agrees about which components exist.

Use `-format=okf` when rendering an [Open Knowledge Format](https://github.com/GoogleCloudPlatform/knowledge-catalog/tree/main/okf) bundle. It rewrites OKF root-relative Markdown links such as `/tables/orders.md` for the rendered site. It is presentation support only; validate bundles with an OKF-aware tool such as `specmd validate`.

The command is a thin wrapper around package github.com/tmc/md2html.
