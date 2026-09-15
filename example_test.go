package md2html_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tmc/md2html"
)

// Rendering a Markdown string is a single call.
func Example() {
	frag, err := md2html.RenderFragment("# Title\n\nSome *text*.\n", "", md2html.FragmentOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(frag.HTML)
	// Output:
	// <h1 id="title">Title</h1>
	// <p>Some <em>text</em>.</p>
}

// A fragment reports the client-side enhancements its HTML needs, so a
// page embeds MathJax or Mermaid only when something on it uses them.
func ExampleRenderFragment() {
	frag, err := md2html.RenderFragment("```mermaid\ngraph TD;\nA-->B;\n```\n", "", md2html.FragmentOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("mermaid:", frag.HasMermaid)
	fmt.Println("math:", frag.HasMath)
	// Output:
	// mermaid: true
	// math: false
}

// FragmentStyles returns the stylesheet the rendered fragments expect.
// It is fixed, so a page can serve it from its own static assets rather
// than inlining it in every response.
func ExampleFragmentStyles() {
	css := string(md2html.FragmentStyles())
	fmt.Println(strings.Contains(css, ".mermaid"))
	// Output: true
}

// A Loader reads data files relative to a base directory, and hands a
// template the functions to reach them.
func ExampleLoader_TemplateFuncs() {
	dir, err := os.MkdirTemp("", "md2html-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "site.json"), []byte(`{"name":"docs"}`), 0o644); err != nil {
		log.Fatal(err)
	}

	loader := md2html.NewLoader(dir, ".html")
	funcs := loader.TemplateFuncs()
	names := make([]string, 0, len(funcs))
	for name := range funcs {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println(names)
	// Output: [load loadJSON loadMD loadYAML]
}

// ParseSummary turns a SUMMARY.md into the navigation tree the renderer
// uses for the sidebar and for prev/next links. An item keeps both the
// source it came from and the URL it renders to.
func ExampleParseSummary() {
	nav, err := md2html.ParseSummary("# Summary\n\n- [Intro](intro.md)\n- [Guide](guide.md)\n", ".html")
	if err != nil {
		log.Fatal(err)
	}
	for _, item := range nav.Flat {
		fmt.Println(item.Title, item.Path, item.URL)
	}
	// Output:
	// Intro intro.md intro.html
	// Guide guide.md guide.html
}

// LoadNavigationOrAutoFromDir prefers a SUMMARY.md and falls back to the
// shape of the tree itself, so a directory with no summary still gets a
// sidebar.
func ExampleLoadNavigationOrAutoFromDir() {
	dir, err := os.MkdirTemp("", "md2html-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{"index.md", "guide.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# "+name+"\n"), 0o644); err != nil {
			log.Fatal(err)
		}
	}

	nav, err := md2html.LoadNavigationOrAutoFromDir(dir, ".html")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(nav.Flat) > 0)
	// Output: true
}
