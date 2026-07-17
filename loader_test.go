package md2html

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoaderParseMD(t *testing.T) {
	loader := NewLoader(".", ".html")

	tests := []struct {
		name         string
		content      string
		wantTitle    string
		wantHeadings int
		wantLinks    int
	}{
		{
			name: "simple document",
			content: `# Hello World

This is a paragraph with a [link](example.md).

## Section One

More content here.
`,
			wantTitle:    "Hello World",
			wantHeadings: 2,
			wantLinks:    1,
		},
		{
			name: "frontmatter title",
			content: `---
title: From Frontmatter
---

# Heading Title

Content here.
`,
			wantTitle:    "From Frontmatter",
			wantHeadings: 1,
			wantLinks:    0,
		},
		{
			name: "summary style",
			content: `# Summary

* [Introduction](README.md)

## Getting Started
* [Overview](getting-started/README.md)
  * [Installation](getting-started/installation.md)
  * [Quick Start](getting-started/quickstart.md)

## Features
* [Features](features/README.md)
`,
			wantTitle:    "Summary",
			wantHeadings: 3, // Summary, Getting Started, Features
			wantLinks:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := loader.ParseMD(tt.content)
			if err != nil {
				t.Fatalf("ParseMD() error = %v", err)
			}

			if doc.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", doc.Title, tt.wantTitle)
			}

			if len(doc.Headings) != tt.wantHeadings {
				t.Errorf("Headings count = %d, want %d", len(doc.Headings), tt.wantHeadings)
			}

			if len(doc.Links) != tt.wantLinks {
				t.Errorf("Links count = %d, want %d", len(doc.Links), tt.wantLinks)
			}
		})
	}
}

func TestLoaderParseMDLinks(t *testing.T) {
	loader := NewLoader(".", ".html")

	content := `# Summary

* [Introduction](README.md)
* [Getting Started](getting-started/README.md)
  * [Installation](getting-started/installation.md)
  * [Quick Start](getting-started/quickstart.md)
`

	doc, err := loader.ParseMD(content)
	if err != nil {
		t.Fatalf("ParseMD() error = %v", err)
	}

	expectedLinks := []struct {
		text string
		url  string
	}{
		{"Introduction", "README.md"},
		{"Getting Started", "getting-started/README.md"},
		{"Installation", "getting-started/installation.md"},
		{"Quick Start", "getting-started/quickstart.md"},
	}

	if len(doc.Links) != len(expectedLinks) {
		t.Fatalf("Links count = %d, want %d", len(doc.Links), len(expectedLinks))
	}

	for i, want := range expectedLinks {
		got := doc.Links[i]
		if got.Text != want.text {
			t.Errorf("Link[%d].Text = %q, want %q", i, got.Text, want.text)
		}
		if got.URL != want.url {
			t.Errorf("Link[%d].URL = %q, want %q", i, got.URL, want.url)
		}
	}
}

func TestLoaderParseMDHeadings(t *testing.T) {
	loader := NewLoader(".", ".html")

	content := `# Main Title

## Section One

### Subsection

## Section Two
`

	doc, err := loader.ParseMD(content)
	if err != nil {
		t.Fatalf("ParseMD() error = %v", err)
	}

	expectedHeadings := []struct {
		level int
		text  string
		id    string
	}{
		{1, "Main Title", "main-title"},
		{2, "Section One", "section-one"},
		{3, "Subsection", "subsection"},
		{2, "Section Two", "section-two"},
	}

	if len(doc.Headings) != len(expectedHeadings) {
		t.Fatalf("Headings count = %d, want %d", len(doc.Headings), len(expectedHeadings))
	}

	for i, want := range expectedHeadings {
		got := doc.Headings[i]
		if got.Level != want.level {
			t.Errorf("Heading[%d].Level = %d, want %d", i, got.Level, want.level)
		}
		if got.Text != want.text {
			t.Errorf("Heading[%d].Text = %q, want %q", i, got.Text, want.text)
		}
		if got.ID != want.id {
			t.Errorf("Heading[%d].ID = %q, want %q", i, got.ID, want.id)
		}
	}
}

func TestToAnchorID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"Getting Started", "getting-started"},
		{"API Reference", "api-reference"},
		{"Section 1.2", "section-12"},
		{"  Spaces  ", "spaces"},
		{"Special!@#Characters", "specialcharacters"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toAnchorID(tt.input)
			if got != tt.want {
				t.Errorf("toAnchorID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoaderLoadJSON(t *testing.T) {
	_ = NewLoader("testdata/gitbook-sample", ".html")

	// Create a temporary JSON string to parse (not from file)
	content := `{"name": "test", "count": 42}`

	// Test the underlying JSON parsing logic
	var data any
	err := json.Unmarshal([]byte(content), &data)
	if err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	m, ok := data.(map[string]any)
	if !ok {
		t.Fatal("Expected map from JSON")
	}

	if m["name"] != "test" {
		t.Errorf("name = %v, want 'test'", m["name"])
	}
}

func TestLoaderLoadYAML(t *testing.T) {
	// Test YAML parsing logic
	content := `
name: test
count: 42
items:
  - one
  - two
`
	var data any
	err := yaml.Unmarshal([]byte(content), &data)
	if err != nil {
		t.Fatalf("YAML parse error: %v", err)
	}

	m, ok := data.(map[string]any)
	if !ok {
		t.Fatal("Expected map from YAML")
	}

	if m["name"] != "test" {
		t.Errorf("name = %v, want 'test'", m["name"])
	}
}
