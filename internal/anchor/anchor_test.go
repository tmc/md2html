package anchor

import "testing"

func TestID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"words", "Hello World", "hello-world"},
		{"dot separates", "Section 1.2", "section-1-2"},
		{"surrounding space", "  Spaces  ", "spaces"},
		{"non-ascii kept", "Überblick", "Überblick"},
		{"empty", "", ""},
		{"punctuation only", "!?.", ""},

		// Measured against `mint dev` on a probe page, heading text to
		// the id Mintlify put in the DOM. These are the contract: md2html
		// is the second renderer of the same source, so a deep link that
		// works on one has to work on the other.
		{
			"slashes and underscores survive",
			"click/type_text/hover/focus/press_key timeout units ambiguous",
			"click/type_text/hover/focus/press_key-timeout-units-ambiguous",
		},
		{
			"hyphen separator meeting a code span's leading hyphen",
			"click/type_text/hover - `-har` flag",
			"click/type_text/hover-har-flag",
		},
		{
			"em dash separator keeps the dash and one hyphen either side",
			"click/type_text/hover — `-har` flag",
			"click/type_text/hover-—-har-flag",
		},
		{"a run of hyphens vanishes", "a --- b", "a-b"},
		{"two hyphens are an em dash, as the rendered text shows", "a -- b", "a-—-b"},
		{"four hyphens are left alone", "a ---- b", "a-b"},
		{
			"leading punctuation of a code span does not double the hyphen",
			"0d. `-har` is discarded",
			"0d-har-is-discarded",
		},
		{
			"em dash is kept as written",
			"cdp — generalized browser scripting",
			"cdp-—-generalized-browser-scripting",
		},
		{"underscore, slash, and equals are literal", "a_b/c=d ratio", "a_b/c=d-ratio"},
		{"trailing separator stripped", "trailing dash -", "trailing-dash"},
		{"leading separator stripped", "- leading dash", "leading-dash"},
		{"whitespace runs collapse", "double  space", "double-space"},
		{"dots separate rather than vanish", "dot.separated.words", "dot-separated-words"},
		{"brackets separate", "paren (thing) and [bracket]", "paren-thing-and-bracket"},
		{"plus and ampersand survive", "plus+plus & amp", "plus+plus-&-amp"},
		{
			"deleting separators must not weld words",
			"extension_console/extension_evaluate",
			"extension_console/extension_evaluate",
		},

		{"trailing punctuation trimmed", "Exit status:", "exit-status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ID(tt.in); got != tt.want {
				t.Errorf("ID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIDsAreUniquePerDocument(t *testing.T) {
	ids := NewIDs()
	want := []string{"setup", "setup-1", "setup-2"}
	for i, w := range want {
		got := string(ids.Generate([]byte("Setup"), 0))
		if got != w {
			t.Errorf("heading %d: got %q, want %q", i, got, w)
		}
	}

	// A heading with no id-able characters still gets an anchor.
	if got := string(ids.Generate([]byte("!!!"), 0)); got != "heading" {
		t.Errorf("punctuation-only heading = %q, want %q", got, "heading")
	}

	// A separate document starts over.
	if got := string(NewIDs().Generate([]byte("Setup"), 0)); got != "setup" {
		t.Errorf("fresh document = %q, want %q", got, "setup")
	}
}

func TestIDsPutReservesAnID(t *testing.T) {
	ids := NewIDs()
	ids.Put([]byte("setup"))
	if got := string(ids.Generate([]byte("Setup"), 0)); got != "setup-1" {
		t.Errorf("got %q, want %q — the id was already taken", got, "setup-1")
	}
}
