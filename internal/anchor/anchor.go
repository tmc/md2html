// Package anchor generates the element ids used for heading anchors.
//
// One algorithm serves the renderer, the document loader, and mdvet, so
// a deep link that vet accepts is one the rendered page answers.
package anchor

import (
	"strconv"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

// ID converts heading text to an anchor id.
//
// The rules follow Mintlify, which publishes these docs; md2html is the
// second renderer, and a link that resolves in one has to resolve in the
// other. Concretely that means "/", "_", and "=" survive rather than
// being deleted — dropping them welds words together, turning
// "extension_console/extension_evaluate" into
// "extension-consoleextension-evaluate" — and non-ASCII characters,
// em dashes in particular, are kept as written.
//
// The rule is otherwise uniform: ASCII letters lowercase, digits and the
// preserved punctuation stay, and everything else becomes a separator.
// Runs of separators collapse to a single "-", which is then trimmed
// from both ends. Collapsing is what absorbs the leading punctuation of
// an inline-code span, so "0d. `-har` is discarded" yields
// "0d-har-is-discarded" rather than doubling the hyphen.
//
// Mapping the rest to a separator rather than deleting it is what makes
// "dot.separated.words" read as "dot-separated-words"; where the
// character already sits next to a space, as in "paren (thing)",
// collapsing makes the two indistinguishable.
func ID(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b = append(b, c+('a'-'A'))
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b = append(b, c)
		case c == '/', c == '_', c == '=', c == '+', c == '&':
			b = append(b, c)
		case c >= 0x80:
			// Part of a multi-byte rune; pass it through untouched.
			// Em dashes in particular survive into the id.
			b = append(b, c)
		default:
			// A separator only where there is something to separate,
			// which drops leading punctuation and collapses runs.
			if len(b) > 0 && b[len(b)-1] != '-' {
				b = append(b, '-')
			}
		}
	}
	for len(b) > 0 && b[len(b)-1] == '-' {
		b = b[:len(b)-1]
	}
	return string(b)
}

// ids assigns every heading in one document a unique id.
type ids struct {
	used map[string]bool
}

// NewIDs returns the id generator goldmark should use while parsing a
// single document. Each document needs its own: the generator remembers
// what it has handed out, so a repeated heading gets "-1", "-2", and so
// on rather than a duplicate.
func NewIDs() parser.IDs {
	return &ids{used: make(map[string]bool)}
}

// Generate implements [parser.IDs].
func (s *ids) Generate(value []byte, kind ast.NodeKind) []byte {
	id := ID(string(value))
	if id == "" {
		// A heading of nothing but punctuation still needs an anchor.
		id = "heading"
	}
	if !s.used[id] {
		s.used[id] = true
		return []byte(id)
	}
	for i := 1; ; i++ {
		candidate := id + "-" + strconv.Itoa(i)
		if !s.used[candidate] {
			s.used[candidate] = true
			return []byte(candidate)
		}
	}
}

// Put implements [parser.IDs].
func (s *ids) Put(value []byte) {
	s.used[string(value)] = true
}
