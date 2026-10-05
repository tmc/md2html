package include

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree writes files, keyed by slash-separated path, under a new
// directory and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const mainGo = `package main

import "fmt"

func main() {
	// region greet
	name := "gopher"
	if name != "" {
		fmt.Println("hello", name)
	}
	// endregion greet
}
`

func TestExpand(t *testing.T) {
	root := writeTree(t, map[string]string{
		"docs/guide.md":          "",
		"docs/snippet.py":        "print('hi')\n",
		"examples/hello/main.go": mainGo,
		"examples/ticks.md":      "a\n```go\nx\n```\n",
		"examples/Makefile":      "all:\n",
		"examples/noeol.txt":     "no newline",
		"examples/prose.c":       "// region of memory to scan\nint *p;\n",
	})
	r := &Resolver{
		Root: root,
		Link: func(path string, first, last int) string {
			if first == 0 {
				return "https://example.com/" + path
			}
			return fmt.Sprintf("https://example.com/%s#L%d-L%d", path, first, last)
		},
	}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"relative",
			"Intro.\n\n<Code src=\"snippet.py\" />\n\nAfter.\n",
			"Intro.\n\n```python\nprint('hi')\n```\n\nAfter.\n",
		},
		{
			"root absolute",
			"<Code src=\"/examples/Makefile\" />\n",
			"```makefile\nall:\n```\n",
		},
		{
			"parent directory",
			"<Code src='../examples/noeol.txt' lang=text/>\n",
			"```text\nno newline\n```\n",
		},
		{
			"whole file drops markers",
			"<Code src=\"/examples/hello/main.go\" />\n",
			"```go\n" + strings.NewReplacer("\t// region greet\n", "", "\t// endregion greet\n", "").Replace(mainGo) + "```\n",
		},
		{
			"whole file keeps prose",
			"<Code src=\"/examples/prose.c\" />\n",
			"```c\n// region of memory to scan\nint *p;\n```\n",
		},
		{
			"region",
			"<Code src=\"/examples/hello/main.go\" region=\"greet\" />\n",
			"```go\nname := \"gopher\"\nif name != \"\" {\n\tfmt.Println(\"hello\", name)\n}\n```\n",
		},
		{
			"title",
			"<Code src=\"snippet.py\" title='Say \"hi\"' />\n",
			"```python {title=\"Say 'hi'\"}\nprint('hi')\n```\n",
		},
		{
			"title with backticks",
			"<Code src=\"/examples/ticks.md\" title=\"The `go` fence\" />\n",
			"~~~markdown {title=\"The `go` fence\"}\na\n```go\nx\n```\n~~~\n",
		},
		{
			"link",
			"<Code src=\"snippet.py\" link />\n",
			"```python {title=\"docs/snippet.py\" href=\"https://example.com/docs/snippet.py\"}\nprint('hi')\n```\n",
		},
		{
			"link to region",
			"<Code\n  src=\"/examples/hello/main.go\"\n  region=\"greet\"\n  title=\"Greeting\"\n  link={true}\n/>\n",
			"```go {title=\"Greeting\" href=\"https://example.com/examples/hello/main.go#L7-L10\"}\nname := \"gopher\"\nif name != \"\" {\n\tfmt.Println(\"hello\", name)\n}\n```\n",
		},
		{
			"longer fence",
			"<Code src=\"/examples/ticks.md\" />\n",
			"````markdown\na\n```go\nx\n```\n````\n",
		},
		{
			"indented",
			"- item\n\n  <Code src=\"snippet.py\" />\n",
			"- item\n\n  ```python\n  print('hi')\n  ```\n",
		},
		{
			"inside a fence",
			"```mdx\n<Code src=\"snippet.py\" />\n```\n",
			"```mdx\n<Code src=\"snippet.py\" />\n```\n",
		},
		{
			"other tags",
			"<CodeGroup>\n<Codec />\n",
			"<CodeGroup>\n<Codec />\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, errs := r.Expand(filepath.Join(root, "docs"), []byte(tt.in))
			if len(errs) > 0 {
				t.Fatalf("errs = %v", errs)
			}
			if string(out) != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", out, tt.want)
			}
		})
	}
}

func TestExpandFiles(t *testing.T) {
	root := writeTree(t, map[string]string{"a.txt": "a\n", "sub/b.txt": "b\n"})
	r := &Resolver{Root: root}
	_, files, errs := r.Expand(root, []byte("<Code src=\"a.txt\" />\n<Code src=\"sub/b.txt\" />\n<Code src=\"gone.txt\" />\n"))
	want := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "sub", "b.txt"), filepath.Join(root, "gone.txt")}
	if strings.Join(files, "\n") != strings.Join(want, "\n") {
		t.Errorf("files = %q, want %q", files, want)
	}
	if len(errs) != 1 || errs[0].Line != 3 {
		t.Errorf("errs = %v, want one on line 3", errs)
	}
}

func TestExpandErrors(t *testing.T) {
	root := writeTree(t, map[string]string{
		"site/page.md":      "",
		"site/main.go":      mainGo,
		"site/twice.go":     "// region a\n// region a\nx\n// endregion a\n",
		"site/open.go":      "// region a\nx\n",
		"site/empty.go":     "// region a\n// endregion a\n",
		"site/binary.bin":   "a\x00b",
		"site/big.txt":      strings.Repeat("x", maxSize+1),
		"site/.env":         "SECRET=1\n",
		"site/.git/config":  "[core]\n",
		"site/private/k.go": "package k\n",
		"site/dir/f.txt":    "f\n",
		"secret.txt":        "outside\n",
	})
	site := filepath.Join(root, "site")
	if err := os.Symlink(filepath.Join(root, "secret.txt"), filepath.Join(site, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(site, "up")); err != nil {
		t.Fatal(err)
	}
	r := &Resolver{
		Root: site,
		Excluded: func(path string) bool {
			return strings.Contains(path, "private")
		},
	}
	tests := []struct {
		tag  string
		want string
	}{
		{`<Code />`, "missing src"},
		{`<Code src="" />`, "missing src"},
		{`<Code src="gone.go" />`, "gone.go: no such file"},
		{`<Code src="../secret.txt" />`, "outside the include root"},
		{`<Code src="/../secret.txt" />`, "secret.txt: no such file"},
		{`<Code src="link.txt" />`, "link.txt: "},
		{`<Code src="up/secret.txt" />`, "up/secret.txt: "},
		{`<Code src=".env" />`, "hidden files cannot be included"},
		{`<Code src=".git/config" />`, "hidden files cannot be included"},
		{`<Code src="private/k.go" />`, "excluded by the ignore file"},
		{`<Code src="dir" />`, "not a regular file"},
		{`<Code src="binary.bin" />`, "not a text file"},
		{`<Code src="big.txt" />`, "larger than 256 KiB"},
		{`<Code src="https://example.com/x.go" />`, "not a file path"},
		{`<Code src="main.go" region="nope" />`, "no region nope"},
		{`<Code src="main.go" region="" />`, "empty region name"},
		{`<Code src="twice.go" region="a" />`, "region a begins twice"},
		{`<Code src="open.go" region="a" />`, "region a does not end"},
		{`<Code src="empty.go" region="a" />`, "region a is empty"},
		{`<Code src="main.go" link />`, "link needs a repository"},
		{`<Code src="main.go" link="maybe" />`, "link must be true or false"},
		{`<Code src="main.go" symbol="main" />`, "unknown attribute symbol"},
		{`<Code src="main.go" lines="1-2" />`, "unknown attribute lines"},
		{`<Code src="main.go" src="main.go" />`, "duplicate attribute src"},
		{`<Code src="main.go">`, `write the tag on its own`},
		{`<Code src="main.go" /> and more`, `write the tag on its own`},
		{`<Code src=main.go`, "unterminated tag"},
		{`<Code src="main.go" =x />`, "malformed attribute"},
	}
	for _, tt := range tests {
		out, _, errs := r.Expand(site, []byte("# Page\n\n"+tt.tag+"\n"))
		if len(errs) != 1 {
			t.Errorf("%s: errs = %v, want one", tt.tag, errs)
			continue
		}
		if errs[0].Line != 3 || !strings.Contains(errs[0].Msg, tt.want) {
			t.Errorf("%s: error %q, want line 3 and %q", tt.tag, errs[0], tt.want)
		}
		if !strings.Contains(string(out), "> [!CAUTION]\n> Code: "+errs[0].Msg) {
			t.Errorf("%s: output does not show the error:\n%s", tt.tag, out)
		}
		if strings.Contains(string(out), "outside") && !strings.Contains(tt.want, "outside") {
			t.Errorf("%s: output leaks a file from outside the root:\n%s", tt.tag, out)
		}
		if strings.Contains(errs[0].Msg, root) {
			t.Errorf("%s: error names an absolute path: %s", tt.tag, errs[0].Msg)
		}
	}
}

func TestExpandDisabled(t *testing.T) {
	root := writeTree(t, map[string]string{"a.txt": "a\n"})
	for _, tt := range []struct {
		name string
		r    *Resolver
		dir  string
	}{
		{"nil resolver", nil, root},
		{"no root", &Resolver{}, root},
		{"no page directory", &Resolver{Root: root}, ""},
	} {
		_, files, errs := tt.r.Expand(tt.dir, []byte("<Code src=\"a.txt\" />\n"))
		if len(errs) != 1 || len(files) != 0 {
			t.Errorf("%s: files = %v, errs = %v, want one error and no files", tt.name, files, errs)
		}
	}
	// Without a tag there is nothing to refuse.
	src := []byte("plain\n")
	if out, _, errs := (*Resolver)(nil).Expand("", src); string(out) != "plain\n" || errs != nil {
		t.Errorf("no tag: out = %q, errs = %v", out, errs)
	}
}

func TestRegionMarkers(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"hash", "# region a\nx = 1\n# endregion a\n", "x = 1\n"},
		{"sql", "-- region a\nSELECT 1;\n-- endregion\n", "SELECT 1;\n"},
		{"html", "<!-- region a -->\n<p>x</p>\n<!-- endregion a -->\n", "<p>x</p>\n"},
		{"css", "/* region a */\na { }\n/* endregion a */\n", "a { }\n"},
		{"editor", "// #region a\nx()\n// #endregion a\n", "x()\n"},
		{"nested", "// region a\nx()\n// region b\ny()\n// endregion b\nz()\n// endregion a\n", "x()\ny()\nz()\n"},
		{"dedent", "\t// region a\n\tif x {\n\n\t\ty()\n\t}\n\t// endregion a\n", "if x {\n\n\ty()\n}\n"},
		{"nested unnamed end", "// region a\nx()\n// region b\ny()\n// endregion\nz()\n// endregion a\n", "x()\ny()\nz()\n"},
		{"prose", "// region a\n// region of memory to scan\nx()\n// endregion a\n", "// region of memory to scan\nx()\n"},
		{"prefix name", "// region ab\nno\n// endregion ab\n// region a\nyes\n// endregion a\n", "yes\n"},
	}
	for _, tt := range tests {
		got, _, _, err := region([]byte(tt.data), "a")
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLanguage(t *testing.T) {
	tests := []struct{ name, want string }{
		{"main.go", "go"},
		{"go.mod", "go.mod"},
		{"a/b/app.PY", "python"},
		{"config.yml", "yaml"},
		{"Makefile", "makefile"},
		{"Dockerfile", "dockerfile"},
		{"query.sql", "sql"},
		{".bashrc", "bashrc"},
	}
	for _, tt := range tests {
		if got := language(tt.name); got != tt.want {
			t.Errorf("language(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
