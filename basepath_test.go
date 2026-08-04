package md2html

import "testing"

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"/", "", false},
		{"/docs", "/docs", false},
		{"docs", "/docs", false},
		{"/docs/", "/docs", false},
		{" /docs/guide ", "/docs/guide", false},
		{"https://example.com/docs", "", true},
	}
	for _, tt := range tests {
		got, err := normalizeBasePath(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("normalizeBasePath(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("normalizeBasePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
