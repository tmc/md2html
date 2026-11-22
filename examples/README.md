# Template examples

Each directory here is a template set for `md2html -templates`. `md2html`
parses its embedded templates first and then the `.html` files in the named
directory, so any `{{define "..."}}` block replaces the embedded template of
that name while all other templates keep their embedded definitions.

```sh
md2html -http :8080 -templates examples/auto-appearance-templates
```

## auto-appearance-templates

Overrides `styles` (`styles.html`) with GitHub Primer colors for light and dark
themes, selected via `prefers-color-scheme`.

## dark-mode-templates

Overrides `styles` (`styles.html`) with GitHub's dark theme colors regardless
of system preference, including scrollbar styling.

## primer-cdn-templates

Overrides `layout`, `styles`, and `content` (`layout.html`, `styles.html`,
`content.html`) with a single-column page styled by Primer CSS loaded from
unpkg.com. Pages rendered with this set require network access for styles.

## Writing a template set

Create a directory containing only the templates you want to override. Most
sets need only `styles.html`. See [../templates](../templates) for the embedded
template names and the data each template uses.
