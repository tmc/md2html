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

Navigation and component icons work without installation. md2html embeds pinned Font Awesome Free, Lucide, and Tabler Outline sets and follows `docs.json`'s `icons.library`; the Mintlify default is Font Awesome. Font Awesome's `iconType` supports the free `solid`, `regular`, and `brands` styles. An unavailable icon is left blank and reported once.

Use `-icons dir` to replace the selected library with a directory of SVG files named for the icons that request them, or `-no-icons` to disable icons. A project-local `icons` directory is also a complete replacement. md2html does not consult a user-global icon directory or fetch icons while rendering, so the same source tree renders identically on different machines.

`-github-stars` shows the star count of the repository named in `docs.json` beside the repository link. The count is fetched at build time and refreshed in the browser, so a deployed page does not show the number frozen at the build; the fetch is cached for five minutes per reader. Without the flag no page makes any request.

`-jsonspec dir` labels JSON examples with the schema they follow. A fenced `json` block whose `"type"` value starts with one of the prefixes in `dir/jsonspec.json` gets a badge naming its type, and the fields of the matching `dir/<type>.schema.json` show their descriptions on hover:

	{"prefixes": ["acme/"], "badge_url": "schemas.html#%s", "badge_label": "%s schema"}

With that file, a block containing `"type": "acme/order"` is labeled "order schema", links to `schemas.html#order`, and takes its field descriptions from `order.schema.json`. The server also serves the loaded schemas at `/_jsonspec/schemas.json`.

Pages a repository keeps out of its site are listed in a `.md2htmlignore`, in gitignore syntax, at or above the directory being served; a `.mintignore` is read under the same rules, so a tree that already declares its exclusions does not have to repeat them.

Generate static output with agent-readable summaries:

	md2html -html _site -llms .

Markdown supports GitHub-style alerts such as "> \[!NOTE]" and fenced admonitions such as "!!!note". Image syntax pointing at audio or video files, for example "!\[demo](demo.mp4)", renders as media controls.

Rendering follows GFM, so a single newline is a space and paragraphs reflow to the viewport. Subscript ("H\~2\~O"), superscript ("X^2^"), and definition lists are not supported. Raw HTML is dropped by default; `-allow-unsafe` passes it through, along with attribute syntax and link rewriting inside HTML blocks.

Docs that link to themselves with root-absolute paths, such as `/docs/quickstart`, only resolve when the tree is served under the prefix those links assume. Use `-base` to preview such a subtree:

	cd docs && md2html -http :8080 -base /docs

Pages are then served under `/docs/`, and a request outside the prefix redirects into it. Serving the parent directory instead works whenever the directory name already matches the prefix.

Served pages take their title from the frontmatter `title`, then the first heading, then the file name. A directory URL serves that directory's index file (`index.md` or `README.md`, or the file named by `-index`) and otherwise a listing of the Markdown files beneath it, to `-depth` levels. A static build chooses the root `index.html` the same way.

Static output carries the files pages point at, keeping their paths: images, media, fonts, stylesheets and scripts, PDFs, and what a host asks for by name such as `robots.txt`, `favicon.ico`, and a web app manifest. The set is an allowlist rather than everything that is not Markdown, because a docs tree usually sits inside a repository holding source and configuration that has no business on a web host. Dot files, the output directory, and anything the ignore file excludes stay out. The development server serves whatever it finds, so a page that renders locally now deploys the same way.

Every page carries the metadata a link preview is built from: `description`, `rel=canonical`, `og:title`, `og:description`, `og:type` (`website` at the site root, `article` elsewhere), `og:site_name`, `og:url`, and `twitter:card`. Set `-site-url` so the canonical URL and `og:url` are absolute; a crawler has no document to resolve a relative one against. A page names its card image with frontmatter `og_image` (or `image`) and describes it with `og_image_alt`, and `-og-image` supplies one for every page that names none. A page image resolves against the page URL and the `-og-image` default against the site root, so both are emitted absolute. Pages with an image advertise `summary_large_image`, the large card X and Slack draw from a 1200×630 image; pages without advertise `summary`.

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
