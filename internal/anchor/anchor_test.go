package anchor

import "testing"

func TestID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"words", "Hello World", "hello-world"},
		{"punctuation", "Section 1.2", "section-12"},
		{"spacing", "  Spaces  ", "--spaces--"},
		{"unicode", "Überblick", "Überblick"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ID(tt.in); got != tt.want {
				t.Fatalf("ID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
