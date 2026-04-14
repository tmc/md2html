package md2html

import (
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
