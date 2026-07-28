// Package anchor implements Markdown heading identifiers.
package anchor

import "strings"

// ID returns the identifier Goldmark assigns to a heading before adding
// duplicate suffixes.
func ID(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		v := s[i]
		if v >= 0x80 {
			b.WriteByte(v)
			continue
		}
		switch {
		case v >= 'A' && v <= 'Z':
			b.WriteByte(v + ('a' - 'A'))
		case v >= 'a' && v <= 'z', v >= '0' && v <= '9':
			b.WriteByte(v)
		case v == ' ' || v == '\t' || v == '-' || v == '_':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), " \t")
}
