package docversions

import (
	"slices"
	"testing"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v0.1.10", "v0.1.9", +1},
		{"v0.1.9", "v0.1.10", -1},
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v1.0.0-rc.1", +1},
		{"v1.0.0-rc.2", "v1.0.0-rc.10", -1},
		{"v1.0.0-rc.1", "v1.0.0-beta", +1},
		{"v1.0.0-1", "v1.0.0-alpha", -1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.2", "v1.1.9", +1},
		{"1.2.3", "v1.2.2", +1},
		{"v1.0.0", "main", +1},
		{"main", "develop", +1},
		{"v01.0.0", "v1.0.0", -1}, // leading zero is not semver
	}
	for _, tt := range tests {
		if got := Compare(tt.a, tt.b); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSortNewestFirst(t *testing.T) {
	got := []string{"v0.1.9", "v0.1.10", "v0.1.10-rc1", "v0.2.0", "main", "v0.1.1"}
	SortNewestFirst(got)
	want := []string{"v0.2.0", "v0.1.10", "v0.1.10-rc1", "v0.1.9", "v0.1.1", "main"}
	if !slices.Equal(got, want) {
		t.Errorf("SortNewestFirst = %v, want %v", got, want)
	}
}
