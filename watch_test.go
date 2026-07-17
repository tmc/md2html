package md2html

import "testing"

func TestWatchEnabled(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		source  string
		want    bool
		wantErr bool
	}{
		{"auto directory listing", "auto", "", true, false},
		{"auto source file", "auto", "README.md", true, false},
		{"auto stdin", "auto", "-", false, false},
		{"empty mode", "", "", true, false},
		{"false", "false", "README.md", false, false},
		{"true stdin", "true", "-", true, false},
		{"invalid", "maybe", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := watchEnabled(tt.mode, tt.source)
			if (err != nil) != tt.wantErr {
				t.Fatalf("watchEnabled() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("watchEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
