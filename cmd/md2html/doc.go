// Command md2html renders Markdown as HTML.
//
// It operates in three modes:
//
//   - render a single Markdown file to stdout
//   - serve a Markdown tree over HTTP with live reload (-http)
//   - generate a static HTML tree (-html)
//
// # Usage
//
// Render a single file to stdout:
//
//	md2html README.md
//
// Serve the current tree with live reload:
//
//	md2html -http :8080
//
// Serve without live-reload file watching:
//
//	md2html -watch=false -http :8080
//
// Preview a subtree whose links assume a published URL prefix:
//
//	cd docs && md2html -http :8080 -base /docs
//
// Pages are then served under /docs/, and a request outside the prefix
// redirects into it. Serving the parent directory instead works whenever
// the directory name already matches the prefix.
//
// Generate static output:
//
//	md2html -html _site .
//
// Generate static output with docs navigation and agent-readable summaries:
//
//	md2html -nav -llms -html _site .
//
// Generate versioned static output, one directory per release tag plus
// the working tree as latest:
//
//	md2html -html _site -versions -version-pattern 'v*' -nav docs
//
// Run "md2html -h" for the full list of flags. Notable general flags
// include -C (resolve relative paths against a directory), -title, -css,
// -toc, -templates, -data-json, -render-frontmatter, -drafts, -search,
// -edit-url, -open, -v, and advisory linting with -vet and -vet-checks
// (diagnostics are logged at warn level and never block rendering).
//
// # Pages and navigation
//
// Files named .md, .markdown, and .mdx are pages. Links to them become
// links to the rendered pages. Pages take their title from frontmatter
// title, then the first heading, then the file name. A directory URL
// serves that directory's index file (index.md, README.md, or the file
// named by -index) and otherwise a listing of the Markdown files beneath
// it, to -depth levels (default 2). A static build chooses the root
// index.html the same way.
//
// Pages a repository keeps out of its site are listed in a .md2htmlignore
// file, in gitignore syntax, at or above the directory being served; a
// .mintignore file is read under the same rules.
//
// With -nav, navigation is loaded from SUMMARY.md, then from a Mintlify
// docs.json covering the tree, then from the shape of the tree itself.
// A docs.json is looked for in the source directory and its parents; its
// page paths resolve against that root, pages outside the served directory
// are skipped, and its name becomes the site title unless -title overrides
// it. A SUMMARY.md link to another site, such as
// [API](https://pkg.go.dev/example.com/x), is shown in the sidebar and
// omitted from previous/next links.
//
// -favicon file names the site icon, copied to the output root and linked
// from every page. Without the flag, the favicon named in docs.json is
// used if present.
//
// -github-stars shows the star count of the repository named in docs.json
// beside the repository link. The count is fetched at build time and
// refreshed in the browser (cached for five minutes per reader). Without
// the flag, no page makes any network request for stars.
//
// # Icons
//
// Navigation and component icons work without installation. md2html embeds
// pinned Font Awesome Free, Lucide, and Tabler Outline sets and follows
// docs.json's icons.library (defaulting to Font Awesome). Font Awesome's
// iconType supports the free solid, regular, and brands styles. An
// unavailable icon is left blank and reported once.
//
// Use -icons dir to replace the selected library with a directory of SVG
// files named for the icons pages request, or -no-icons to disable icons.
// A project-local icons directory is also a complete replacement. md2html
// never consults a user-global icon directory or fetches icons while
// rendering.
//
// # Static output and metadata
//
// A static build writes guide.md as guide.html (or with the extension set
// by -html-ext). With -html-layout dir, it writes guide/index.html instead
// and links to it as guide/, adjusting relative references to sibling
// assets to match. A static build also rewrites root-absolute links to
// pages in the tree, such as /docs/quickstart, relative to each page so
// the output works when hosted under a prefix; paths resolve from the
// directory holding docs.json or the source directory.
//
// Static output carries the files pages point at, keeping their paths:
// images, media, fonts, stylesheets and scripts, PDFs, and host files
// such as robots.txt, favicon.ico, and web app manifests. The set is an
// allowlist so repository source and configuration files stay out; dot
// files, the output directory, and ignored paths are also excluded. The
// development server serves whatever it finds in the tree.
//
// With -llms, static output also emits llms.txt, llms-full.txt, and raw
// Markdown copies alongside rendered pages. Drafts (frontmatter draft:
// true) are skipped unless -drafts is set, and remain excluded from
// llms.txt either way.
//
// Every page carries link-preview metadata: description, rel=canonical,
// og:title, og:description, og:type ("website" at the site root, "article"
// elsewhere), og:site_name, og:url, and twitter:card. Set -site-url so
// canonical and og:url values are absolute. A page names its card image
// with frontmatter og_image (or image) and describes it with og_image_alt;
// -og-image supplies a default resolved against -site-url. Pages with an
// image advertise summary_large_image; pages without advertise summary.
//
// # Versioned documentation
//
// With -versions, each matching git tag (controlled by -version-pattern,
// default "*") and optionally each branch (-version-branches) is built
// from its own commit, checked out into a git worktree, alongside the
// working tree as latest. The output root holds versions.json, a versions/
// index page, redirects from unversioned URLs to the default version
// (-version-default, default latest), and _shared/ for scripts shared
// across versions.
//
// A versions.yaml file in the source directory (or the path given to
// -version-config), read from the working tree only, chooses which tags
// to publish, how to label them, and which prebuilt archives to copy in
// verbatim:
//
//	since: v0.1.17       # publish no older tag, nor prereleases
//	exclude: ["v0.1.19"] # nor these
//	collapse: minor      # list only the newest patch of each minor version
//	labels:
//	  latest: main
//	archive:
//	  - name: v0.1.16
//	    label: v0.1.16 and earlier
//	    path: archive/v0.1.16 # directory, or URL of a .tar.gz
//	banner:
//	  old: "You are reading docs for {version}. The latest is {latest}."
//
// When a page moves, git rename detection or an aliases list in the moved
// page's frontmatter lets the version switcher follow it, and the latest
// version keeps a redirect at the old path. An old version's -edit-url
// link names the page where it is now and is omitted for removed pages.
// -version-cache caches worktrees and finished builds; -versions-only
// builds a comma-separated subset; -version-alias stable=newest-tag
// publishes a copy of the newest release under stable/.
//
// With -http, the working tree is served at the root regardless of
// -version-default, and each other version is served under /v/<name>/ from
// a worktree created on first request.
//
// # Layout components and code includes
//
// Built-in MDX-style layout components are supported for Card, CardGroup
// (alias Columns), Steps/Step, Accordion/AccordionGroup (alias
// Expandable), and Frame:
//
//	<CardGroup cols={2}>
//
//	<Card title="Install" href="/install">
//	Run go install to get started.
//	</Card>
//
//	</CardGroup>
//
// A tag must stand alone on its line and its body is ordinary Markdown.
// There is no JavaScript: import and export are unsupported, and brace
// expressions accept only JSON scalars. Unknown component names, unknown
// attributes, and missing required attributes are reported as warnings
// and left unrendered.
//
// Custom components can be defined with -components dir, where dir holds
// a components.json naming each component's attributes and its
// html/template file:
//
//	{
//	  "components": {
//	    "Note": {
//	      "attrs": ["title"],
//	      "required": ["title"],
//	      "template": "note.html"
//	    }
//	  }
//	}
//
// Each template must reference {{.Content}} exactly once and reads
// attributes from {{.Attrs.<name>}}. Attribute values from Markdown are
// always escaped. A custom component may reuse a built-in name to replace
// it. Pass the same directory to mdvet -components so checks agree.
//
// Code includes embed a file as a fenced code block so documentation
// stays in sync with source code:
//
//	<Code src="../examples/hello/main.go" />
//	<Code src="/examples/hello/main.go" region="greet" title="Greeting" link />
//
// src is relative to the page, or to the include root when prefixed with
// a slash. lang overrides the fence language inferred from the file name.
// region selects only the lines between "region NAME" and "endregion NAME"
// comments, stripping shared indentation and omitting the markers. title
// captions the block, and link turns the caption into a GitHub link using
// navbar.primary in docs.json.
//
// Included files must reside inside the include root (the source directory
// by default, or -include-root dir). Hidden files, ignored files, and
// symlinks escaping the root are refused. An unexpandable tag fails a
// static build and renders an error in server mode; the server reloads a
// page when an included file changes. Search indexes, llms-full.txt, and
// raw Markdown copies written by -llms contain the expanded code.
//
// Any fenced code block that names a language can carry a caption:
//
//	```go {title="main.go"}
//
// # JSON schemas
//
// -jsonspec dir labels fenced json blocks with the schema they follow. A
// block whose "type" value starts with a prefix in dir/jsonspec.json gets
// a badge naming its type, and fields from dir/<type>.schema.json show
// descriptions on hover:
//
//	{"prefixes": ["acme/"], "badge_url": "schemas.html#%s", "badge_label": "%s schema"}
//
// The server also serves the loaded schema bundle at /_jsonspec/schemas.json.
//
// # Markdown dialect
//
// Rendering follows GFM: a single newline is a space so paragraphs reflow
// to the viewport. Beyond GFM, md2html supports footnotes, GitHub-style
// alerts ("> [!NOTE]"), fenced admonitions ("!!!note"), tabs, media
// controls for audio/video image links ("![demo](demo.mp4)"), and Mermaid
// fences. Subscript ("H~2~O"), superscript ("X^2^"), and definition lists
// are not supported.
//
// Raw HTML is dropped by default. -allow-unsafe passes it through and also
// enables attribute syntax and link rewriting inside HTML blocks.
//
// Mermaid diagrams and TeX math are rendered in the browser by scripts
// loaded from a CDN, so those pages need network access on first view.
// When a loader fails, the page shows a warning rather than leaving the
// block silently unrendered.
//
// -format=okf enables presentation support for [Open Knowledge Format]
// bundles, rewriting OKF root-relative Markdown links such as
// /tables/orders.md for the rendered site. Validate bundles with an
// OKF-aware tool such as specmd validate.
//
// The command is a thin wrapper around package [github.com/tmc/md2html].
//
// [Open Knowledge Format]: https://github.com/GoogleCloudPlatform/knowledge-catalog/tree/main/okf
package main
