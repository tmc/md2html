package md2html

import "testing"

func TestFingerprintName(t *testing.T) {
	a := fingerprintName("js/search.js", []byte("body"))
	b := fingerprintName("js/search.js", []byte("body"))
	c := fingerprintName("js/search.js", []byte("other"))
	if a != b {
		t.Fatalf("fingerprintName not deterministic: %q != %q", a, b)
	}
	if a == c {
		t.Fatalf("fingerprintName did not change with content: %q", a)
	}
	if a != "js/search.230d8358.js" {
		t.Fatalf("fingerprintName = %q, want js/search.230d8358.js", a)
	}
}
