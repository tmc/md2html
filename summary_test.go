package md2html

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSummary(t *testing.T) {
	content := `# Summary

* [Introduction](README.md)

## Getting Started
* [Overview](getting-started/README.md)
  * [Installation](getting-started/installation.md)
  * [Quick Start](getting-started/quickstart.md)

## Features
* [Features Overview](features/README.md)
  * [Markdown Syntax](features/markdown.md)
  * [Custom Templates](features/templates.md)
  * [Advanced](features/advanced/README.md)
    * [Versioning](features/advanced/versioning.md)

## CLI Reference
* [Command Line](cli/README.md)
  * [Flags Reference](cli/flags.md)

---

* [Changelog](CHANGELOG.md)
`

	nav, err := ParseSummary(content, ".html")
	if err != nil {
		t.Fatalf("ParseSummary() error = %v", err)
	}

	// Check structure
	if len(nav.Items) != 9 {
		// 1 intro + 3 groups + 3 top items after groups + 1 sep + 1 changelog
		t.Errorf("Top-level items = %d, want 9", len(nav.Items))
	}

	// Check flat list (navigable pages only)
	// Count: intro, overview, install, quickstart, features, markdown, templates, advanced, versioning, cli, flags, changelog = 12
	if len(nav.Flat) != 12 {
		t.Errorf("Flat items = %d, want 12", len(nav.Flat))
		for i, item := range nav.Flat {
			t.Logf("  Flat[%d]: %s (%s)", i, item.Title, item.Path)
		}
	}

	// Check first item
	if nav.Items[0].Title != "Introduction" {
		t.Errorf("First item title = %q, want 'Introduction'", nav.Items[0].Title)
	}
	// README.md keeps its rendered file path.
	if nav.Items[0].URL != "README.html" {
		t.Errorf("First item URL = %q, want 'README.html'", nav.Items[0].URL)
	}

	// Check group
	if !nav.Items[1].IsGroup {
		t.Error("Second item should be a group")
	}
	if nav.Items[1].Title != "Getting Started" {
		t.Errorf("Second item title = %q, want 'Getting Started'", nav.Items[1].Title)
	}
}

func TestParseSummaryNesting(t *testing.T) {
	content := `* [Root](root.md)
  * [Child 1](child1.md)
    * [Grandchild](grandchild.md)
  * [Child 2](child2.md)
`

	nav, err := ParseSummary(content, "")
	if err != nil {
		t.Fatalf("ParseSummary() error = %v", err)
	}

	if len(nav.Items) != 1 {
		t.Fatalf("Top-level items = %d, want 1", len(nav.Items))
	}

	root := nav.Items[0]
	if root.Title != "Root" {
		t.Errorf("Root title = %q, want 'Root'", root.Title)
	}

	if len(root.Children) != 2 {
		t.Fatalf("Root children = %d, want 2", len(root.Children))
	}

	child1 := root.Children[0]
	if child1.Title != "Child 1" {
		t.Errorf("Child 1 title = %q, want 'Child 1'", child1.Title)
	}

	if len(child1.Children) != 1 {
		t.Fatalf("Child 1 children = %d, want 1", len(child1.Children))
	}

	grandchild := child1.Children[0]
	if grandchild.Title != "Grandchild" {
		t.Errorf("Grandchild title = %q, want 'Grandchild'", grandchild.Title)
	}

	child2 := root.Children[1]
	if child2.Title != "Child 2" {
		t.Errorf("Child 2 title = %q, want 'Child 2'", child2.Title)
	}
}

func TestNavigationForPage(t *testing.T) {
	content := `* [Page 1](page1.md)
* [Page 2](page2.md)
* [Page 3](page3.md)
`

	nav, err := ParseSummary(content, ".html")
	if err != nil {
		t.Fatalf("ParseSummary() error = %v", err)
	}

	// Test middle page
	ctx := nav.ForPage("page2.md")
	if !ctx.HasNav {
		t.Error("HasNav should be true")
	}
	if ctx.Current == nil {
		t.Fatal("Current should not be nil")
	}
	if ctx.Current.Title != "Page 2" {
		t.Errorf("Current.Title = %q, want 'Page 2'", ctx.Current.Title)
	}
	if ctx.Prev == nil || ctx.Prev.Title != "Page 1" {
		t.Errorf("Prev should be 'Page 1'")
	}
	if ctx.Next == nil || ctx.Next.Title != "Page 3" {
		t.Errorf("Next should be 'Page 3'")
	}

	// Test first page
	ctx = nav.ForPage("page1.md")
	if ctx.Prev != nil {
		t.Error("First page should have no Prev")
	}
	if ctx.Next == nil || ctx.Next.Title != "Page 2" {
		t.Error("First page Next should be 'Page 2'")
	}

	// Test last page
	ctx = nav.ForPage("page3.md")
	if ctx.Prev == nil || ctx.Prev.Title != "Page 2" {
		t.Error("Last page Prev should be 'Page 2'")
	}
	if ctx.Next != nil {
		t.Error("Last page should have no Next")
	}
}

func TestNavigationBreadcrumb(t *testing.T) {
	content := `* [Root](root.md)
  * [Parent](parent.md)
    * [Current](current.md)
`

	nav, err := ParseSummary(content, ".html")
	if err != nil {
		t.Fatalf("ParseSummary() error = %v", err)
	}

	ctx := nav.ForPage("current.md")
	if len(ctx.Breadcrumb) != 2 {
		t.Fatalf("Breadcrumb length = %d, want 2", len(ctx.Breadcrumb))
	}

	if ctx.Breadcrumb[0].Title != "Root" {
		t.Errorf("Breadcrumb[0] = %q, want 'Root'", ctx.Breadcrumb[0].Title)
	}
	if ctx.Breadcrumb[1].Title != "Parent" {
		t.Errorf("Breadcrumb[1] = %q, want 'Parent'", ctx.Breadcrumb[1].Title)
	}
}

func TestPathToURL(t *testing.T) {
	tests := []struct {
		path    string
		htmlExt string
		want    string
	}{
		{"README.md", ".html", "README.html"},
		{"README.md", "", "README"},
		{"page.md", ".html", "page.html"},
		{"page.md", "", "page"},
		{"dir/README.md", ".html", "dir/README.html"},
		{"dir/README.md", "", "dir/README"},
		{"dir/page.md", ".html", "dir/page.html"},
		{"getting-started/installation.md", ".html", "getting-started/installation.html"},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.htmlExt, func(t *testing.T) {
			got := pathToURL(tt.path, tt.htmlExt)
			if got != tt.want {
				t.Errorf("pathToURL(%q, %q) = %q, want %q", tt.path, tt.htmlExt, got, tt.want)
			}
		})
	}
}

func TestParseSummarySeparator(t *testing.T) {
	content := `* [Page 1](page1.md)

---

* [Page 2](page2.md)
`

	nav, err := ParseSummary(content, ".html")
	if err != nil {
		t.Fatalf("ParseSummary() error = %v", err)
	}

	if len(nav.Items) != 3 {
		t.Fatalf("Items = %d, want 3", len(nav.Items))
	}

	if !nav.Items[1].IsSep {
		t.Error("Second item should be a separator")
	}

	// Separator should not be in flat list
	if len(nav.Flat) != 2 {
		t.Errorf("Flat = %d, want 2 (no separator)", len(nav.Flat))
	}
}

func TestNilNavigation(t *testing.T) {
	var nav *Navigation
	ctx := nav.ForPage("anything.md")

	if ctx.HasNav {
		t.Error("HasNav should be false for nil navigation")
	}
	if ctx.Current != nil {
		t.Error("Current should be nil for nil navigation")
	}
}

func TestAutoNavigationLabelDerivation(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "frontmatter.md", "---\ntitle: Frontmatter Title\n---\n# H1 Title\n")
	writeTestFile(t, dir, "heading.md", "# Heading Title\n")
	writeTestFile(t, dir, "plain-file_name.md", "body\n")

	nav, err := AutoNavigationFromDir(dir)
	if err != nil {
		t.Fatalf("AutoNavigationFromDir() error = %v", err)
	}
	got := titles(nav.Flat)
	want := []string{"Frontmatter Title", "Heading Title", "Plain File Name"}
	if !sameStrings(got, want) {
		t.Fatalf("titles = %v, want %v", got, want)
	}
}

func TestAutoNavigationFilenameLabels(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"01-getting-started.md", "Getting Started"},
		{"intro_to_X.md", "Intro To X"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, dir, tt.name, "body\n")
			nav, err := AutoNavigationFromDir(dir)
			if err != nil {
				t.Fatalf("AutoNavigationFromDir() error = %v", err)
			}
			if got := nav.Flat[0].Title; got != tt.want {
				t.Fatalf("title = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutoNavigationSortOrder(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "weight",
			files: map[string]string{
				"a.md": "---\ntitle: A\nweight: 20\n---\n",
				"b.md": "---\ntitle: B\nweight: 10\n---\n",
			},
			want: []string{"B", "A"},
		},
		{
			name: "sidebar position",
			files: map[string]string{
				"a.md": "---\ntitle: A\nsidebar_position: 2\n---\n",
				"b.md": "---\ntitle: B\nsidebar_position: 1\n---\n",
			},
			want: []string{"B", "A"},
		},
		{
			name: "numeric prefix",
			files: map[string]string{
				"02-setup.md": "body\n",
				"01-intro.md": "body\n",
			},
			want: []string{"Intro", "Setup"},
		},
		{
			name: "lexicographic",
			files: map[string]string{
				"b.md": "body\n",
				"a.md": "body\n",
			},
			want: []string{"A", "B"},
		},
		{
			name: "combined",
			files: map[string]string{
				"z.md":       "---\ntitle: Weight\nweight: 1\n---\n",
				"a.md":       "---\ntitle: Sidebar\nsidebar_position: 1\n---\n",
				"02-two.md":  "body\n",
				"01-one.md":  "body\n",
				"plain-a.md": "body\n",
				"plain-b.md": "body\n",
			},
			want: []string{"Weight", "Sidebar", "One", "Two", "Plain A", "Plain B"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				writeTestFile(t, dir, name, content)
			}
			nav, err := AutoNavigationFromDir(dir)
			if err != nil {
				t.Fatalf("AutoNavigationFromDir() error = %v", err)
			}
			if got := titles(nav.Flat); !sameStrings(got, tt.want) {
				t.Fatalf("titles = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAutoNavigationLandingPages(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "guide/README.md", "# Guide\n")
	writeTestFile(t, dir, "guide/01-install.md", "# Install\n")

	nav, err := AutoNavigationFromDir(dir)
	if err != nil {
		t.Fatalf("AutoNavigationFromDir() error = %v", err)
	}
	if len(nav.Items) != 1 || !nav.Items[0].IsGroup {
		t.Fatalf("items = %#v, want one group", nav.Items)
	}
	if got := nav.Items[0].Title; got != "Guide" {
		t.Fatalf("group title = %q, want Guide", got)
	}
	if _, ok := nav.ByPath["guide/README.md"]; ok {
		t.Fatalf("landing page listed in navigation")
	}
	if got := titles(nav.Flat); !sameStrings(got, []string{"Install"}) {
		t.Fatalf("flat titles = %v, want Install", got)
	}
}

func TestAutoNavigationSkipsHiddenAndOutput(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "visible.md", "# Visible\n")
	writeTestFile(t, dir, ".hidden.md", "# Hidden\n")
	writeTestFile(t, dir, ".private/page.md", "# Private\n")
	writeTestFile(t, dir, "output/page.md", "# Output\n")

	nav, err := AutoNavigationFromDir(dir)
	if err != nil {
		t.Fatalf("AutoNavigationFromDir() error = %v", err)
	}
	if got := titles(nav.Flat); !sameStrings(got, []string{"Visible"}) {
		t.Fatalf("titles = %v, want Visible", got)
	}
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func titles(items []*NavItem) []string {
	var out []string
	for _, item := range items {
		out = append(out, item.Title)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAutoNavigationIcons(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"plain.md", "---\nicon: rocket\n---\n# Plain\n", "rocket"},
		{"hyphen.md", "---\nicon: graduation-cap\n---\n# Hyphen\n", "graduation-cap"},
		{"none.md", "# None\n", ""},
		{"blank.md", "---\nicon: \"\"\n---\n# Blank\n", ""},
		{"spaced.md", "---\nicon: \"  rocket  \"\n---\n# Spaced\n", "rocket"},
		// Names that no icon set would use are dropped rather than
		// carried into the page as an attribute value.
		{"upper.md", "---\nicon: Rocket\n---\n# Upper\n", ""},
		{"path.md", "---\nicon: ../../etc/passwd\n---\n# Path\n", ""},
		{"markup.md", "---\nicon: \"a\\\" onload=alert(1)\"\n---\n# Markup\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, dir, tt.name, tt.body)
			nav, err := AutoNavigationFromDir(dir)
			if err != nil {
				t.Fatalf("AutoNavigationFromDir() error = %v", err)
			}
			if got := nav.Flat[0].Icon; got != tt.want {
				t.Fatalf("icon = %q, want %q", got, tt.want)
			}
		})
	}
}
