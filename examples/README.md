# Template examples

Each directory here is a set of templates to pass to md2html with
`-templates`. md2html parses the embedded templates first and then the
`.html` files in the named directory, so a file that defines a template
(`{{define "styles"}}` and so on) replaces the embedded one of that name,
and every template it leaves alone keeps its embedded definition.

```
md2html -http :8080 -templates examples/auto-appearance-templates
```

## auto-appearance-templates

Replaces `styles` with GitHub Primer colors for both light and dark
themes, choosing between them with the `prefers-color-scheme` media
query.

## dark-mode-templates

Replaces `styles` with GitHub's dark theme colors, used whatever the
system preference, and styles the scrollbar to match.

## primer-cdn-templates

Replaces `layout`, `styles`, and `content` with a single-column page
styled by Primer CSS loaded from unpkg.com, so pages rendered with it
need network access to look right.

## Writing a template set

Make a directory and put in it only the templates you want to change.
Most sets need only `styles.html`. The embedded templates are in
[../templates](../templates) and show the names and data each one uses.
