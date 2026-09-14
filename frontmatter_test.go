package md2html

import (
	"strings"
	"testing"

	"github.com/tmc/md2html/internal/mdvet"
)

// TestFrontmatterParsers characterizes the two frontmatter readers this
// repository contains, so that the difference between them stays a
// decision rather than an accident.
//
// Rendering (parseFrontmatter and [Loader.ParseMD]) reads frontmatter
// with goldmark's meta extension, which is lenient: a block it cannot
// make sense of yields no metadata and no error. Vetting
// ([mdvet.FrontmatterCheck]) reads it with a small hand-written parser,
// because a linter that silently ignored a malformed block would have
// nothing to report. The two therefore disagree on purpose, and are not
// candidates for a shared primitive.
func TestFrontmatterParsers(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// want describes goldmark's view: the metadata keys it
		// extracted and the body it left behind.
		meta map[string]any
		body string
		// diags describes mdvet's view: substrings of the
		// diagnostics it reports, in order.
		diags []string
	}{{
		name: "no frontmatter",
		src:  "# Hi\n\nbody\n",
		meta: map[string]any{},
		body: "# Hi\n\nbody\n",
	}, {
		name: "valid",
		src:  "---\ntitle: T\ndraft: false\n---\n\n# Hi\n",
		meta: map[string]any{"title": "T", "draft": false},
		body: "# Hi\n",
	}, {
		name: "crlf line endings",
		src:  "---\r\ntitle: T\r\n---\r\n\r\n# Hi\r\n",
		meta: map[string]any{"title": "T"},
		body: "# Hi\r\n",
		// mdvet requires a "---\n" prefix and so sees no
		// frontmatter here at all.
	}, {
		name: "empty block",
		src:  "---\n---\n\n# Hi\n",
		meta: map[string]any{},
		body: "# Hi\n",
	}, {
		name: "missing closing delimiter",
		src:  "---\ntitle: T\n\n# Hi\n",
		// goldmark takes the rest of the file as frontmatter,
		// leaving no body; mdvet reports nothing, having found no
		// closing marker to parse up to.
		meta: map[string]any{"title": "T"},
		body: "",
	}, {
		name: "closing delimiter with trailing space",
		src:  "---\ntitle: T\n--- \n\n# Hi\n",
		meta: map[string]any{"title": "T"},
		body: "# Hi\n",
	}, {
		name: "invalid yaml",
		src:  "---\ntitle: [unterminated\n---\n\n# Hi\n",
		// goldmark drops the metadata and hands back the block as
		// body text. Only mdvet says why.
		meta:  map[string]any{},
		body:  "title: [unterminated\n---\n\n# Hi\n",
		diags: []string{"invalid frontmatter"},
	}, {
		name:  "duplicate keys",
		src:   "---\ntitle: A\ntitle: B\n---\n\n# Hi\n",
		meta:  map[string]any{"title": "B"},
		body:  "# Hi\n",
		diags: []string{"already defined"},
	}, {
		name:  "missing title",
		src:   "---\ndraft: true\n---\n\n# Hi\n",
		meta:  map[string]any{"draft": true},
		body:  "# Hi\n",
		diags: []string{"title is missing"},
	}, {
		name:  "draft is not a bool",
		src:   "---\ntitle: T\ndraft: \"yes\"\n---\n\n# Hi\n",
		meta:  map[string]any{"title": "T", "draft": "yes"},
		body:  "# Hi\n",
		diags: []string{"draft must be true or false"},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseFrontmatter(tt.src)
			if err != nil {
				t.Fatalf("parseFrontmatter: %v", err)
			}
			checkMeta(t, "parseFrontmatter", doc.Frontmatter, tt.meta)
			if doc.Content != tt.body {
				t.Errorf("parseFrontmatter body = %q, want %q", doc.Content, tt.body)
			}

			// The loader parses the same source with more
			// extensions enabled; its metadata must not differ.
			md, err := NewLoader("", "").ParseMD(tt.src)
			if err != nil {
				t.Fatalf("ParseMD: %v", err)
			}
			checkMeta(t, "ParseMD", md.Frontmatter, tt.meta)

			diags, err := (mdvet.FrontmatterCheck{}).Check(&mdvet.Document{File: "x.md", Source: []byte(tt.src)})
			if err != nil {
				t.Fatalf("FrontmatterCheck: %v", err)
			}
			if len(diags) != len(tt.diags) {
				t.Fatalf("mdvet reported %d diagnostics, want %d: %v", len(diags), len(tt.diags), diags)
			}
			for i, want := range tt.diags {
				if !strings.Contains(diags[i].Message, want) {
					t.Errorf("mdvet diagnostic %d = %q, want it to mention %q", i, diags[i].Message, want)
				}
			}
		})
	}
}

func checkMeta(t *testing.T, who string, got, want map[string]any) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s metadata = %v, want %v", who, got, want)
		return
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s metadata[%q] = %v, want %v", who, k, got[k], v)
		}
	}
}
