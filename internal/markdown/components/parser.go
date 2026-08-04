package components

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ParseError describes a parse-time failure with source location.
type ParseError struct {
	Line int
	Msg  string

	// UnknownComponent reports that the failure was an unregistered
	// component name, and Known lists the names that were registered.
	// Callers use this to explain how the registry is configured, which
	// the parser itself has no way to know.
	UnknownComponent bool
	Known            []string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("components: line %d: %s", e.Line, e.Msg)
}

// Context keys.
var (
	errorsContextKey = parser.NewContextKey()
	stackContextKey  = parser.NewContextKey()
)

// Errors returns any parse errors recorded on pc.
func Errors(pc parser.Context) []*ParseError {
	errs, _ := pc.Get(errorsContextKey).([]*ParseError)
	return errs
}

func appendError(pc parser.Context, line int, format string, args ...any) {
	addError(pc, &ParseError{Line: line, Msg: fmt.Sprintf(format, args...)})
}

func addError(pc parser.Context, e *ParseError) {
	errs, _ := pc.Get(errorsContextKey).([]*ParseError)
	pc.Set(errorsContextKey, append(errs, e))
}

// The open-tag stack tracks which component is innermost, so that a
// closing tag is matched by the node that opened last rather than by
// any enclosing node of the same name.
func pushOpen(pc parser.Context, n *Node) {
	v, _ := pc.Get(stackContextKey).([]*Node)
	pc.Set(stackContextKey, append(v, n))
}

func popOpen(pc parser.Context) {
	v, _ := pc.Get(stackContextKey).([]*Node)
	if len(v) > 0 {
		pc.Set(stackContextKey, v[:len(v)-1])
	}
}

func peekOpen(pc parser.Context) (*Node, bool) {
	v, _ := pc.Get(stackContextKey).([]*Node)
	if len(v) == 0 {
		return nil, false
	}
	return v[len(v)-1], true
}

// currentLine returns a 1-based line number for diagnostics.
func currentLine(reader text.Reader) int {
	ln, _ := reader.Position()
	return ln + 1
}

// isName reports whether s is a component name: an uppercase letter
// followed by letters and digits. The leading uppercase is what
// separates a component from ordinary HTML, matching JSX.
func isName(s string) bool {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

// scanCloseTag matches a line consisting of only </Name>, returning the
// name.
func scanCloseTag(line []byte) (string, bool) {
	s := strings.TrimRight(string(line), " \t\r\n")
	rest, ok := strings.CutPrefix(s, "</")
	if !ok {
		return "", false
	}
	name, ok := strings.CutSuffix(rest, ">")
	if !ok || !isName(name) {
		return "", false
	}
	return name, true
}

// scanOpenTag matches a line consisting of only <Name attrs> or
// <Name attrs/>. It returns the name and the raw attribute text.
func scanOpenTag(line []byte) (name, attrs string, selfClosing, ok bool) {
	s := strings.TrimRight(string(line), " \t\r\n")
	rest, ok := strings.CutPrefix(s, "<")
	if !ok {
		return "", "", false, false
	}
	body, ok := strings.CutSuffix(rest, ">")
	if !ok {
		return "", "", false, false
	}
	if trimmed, cut := strings.CutSuffix(body, "/"); cut {
		body, selfClosing = trimmed, true
	}
	name = body
	if i := strings.IndexAny(body, " \t"); i >= 0 {
		name, attrs = body[:i], strings.TrimSpace(body[i:])
	}
	if !isName(name) {
		return "", "", false, false
	}
	return name, attrs, selfClosing, true
}

// parseAttrs parses an attribute list of the form name="value",
// name={jsonScalar}, or a bare name meaning "true".
func parseAttrs(s string) (map[string]string, error) {
	attrs := map[string]string{}
	for {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			return attrs, nil
		}
		i := 0
		for i < len(s) && (isNameByte(s[i]) || (i > 0 && s[i] == '-')) {
			i++
		}
		if i == 0 {
			return nil, fmt.Errorf("unexpected %q in attributes", s[:1])
		}
		name := s[:i]
		s = s[i:]
		if _, dup := attrs[name]; dup {
			return nil, fmt.Errorf("duplicate attribute %q", name)
		}
		if !strings.HasPrefix(s, "=") {
			attrs[name] = "true"
			continue
		}
		value, rest, err := parseValue(s[1:])
		if err != nil {
			return nil, fmt.Errorf("attribute %q: %w", name, err)
		}
		attrs[name] = value
		s = rest
	}
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// parseValue parses one attribute value and returns the remaining text.
func parseValue(s string) (value, rest string, err error) {
	if s == "" {
		return "", "", fmt.Errorf("missing value")
	}
	switch s[0] {
	case '"', '\'':
		quote := s[0]
		end := strings.IndexByte(s[1:], quote)
		if end < 0 {
			return "", "", fmt.Errorf("unterminated %c-quoted value", quote)
		}
		return s[1 : 1+end], s[end+2:], nil
	case '{':
		end := strings.IndexByte(s, '}')
		if end < 0 {
			return "", "", fmt.Errorf("unterminated brace expression")
		}
		lit, err := jsonScalar(s[1:end])
		if err != nil {
			return "", "", err
		}
		return lit, s[end+1:], nil
	}
	return "", "", fmt.Errorf("value must be quoted or braced")
}

// jsonScalar converts a brace expression to its text form. Only JSON
// scalars are accepted: without a JavaScript runtime there is nothing
// that could evaluate an expression.
func jsonScalar(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	var v any
	if err := json.Unmarshal([]byte(expr), &v); err != nil {
		return "", fmt.Errorf("expression %q is not a JSON scalar", expr)
	}
	switch v := v.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	}
	return "", fmt.Errorf("expression %q is not a JSON scalar", expr)
}

// blockParser recognises component tags at the start of a line.
type blockParser struct {
	registry Registry
}

func (p *blockParser) Trigger() []byte { return []byte{'<'} }

func (p *blockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos >= len(line) || line[pos] != '<' {
		return nil, parser.NoChildren
	}
	name, attrText, selfClosing, ok := scanOpenTag(line[pos:])
	if !ok {
		// Not a component tag; leave it to the HTML block parser.
		return nil, parser.NoChildren
	}
	comp, known := p.registry.Lookup(name)
	if !known {
		addError(pc, &ParseError{
			Line:             currentLine(reader),
			Msg:              fmt.Sprintf("unknown component <%s>", name),
			UnknownComponent: true,
			Known:            p.registry.Names(),
		})
		return nil, parser.NoChildren
	}
	attrs, err := parseAttrs(attrText)
	if err != nil {
		appendError(pc, currentLine(reader), "<%s>: %v", name, err)
		return nil, parser.NoChildren
	}
	if msgs := comp.validate(name, attrs); len(msgs) > 0 {
		for _, msg := range msgs {
			appendError(pc, currentLine(reader), "%s", msg)
		}
		return nil, parser.NoChildren
	}

	openLine := currentLine(reader)
	reader.Advance(len(line) - 1)
	node := NewNode(name, attrs, selfClosing, openLine)
	if selfClosing {
		return node, parser.NoChildren
	}
	pushOpen(pc, node)
	return node, parser.HasChildren
}

func (p *blockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	n := node.(*Node)
	if n.SelfClosing {
		return parser.Close
	}
	line, seg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return parser.Continue | parser.HasChildren
	}
	name, ok := scanCloseTag(line[pos:])
	if !ok || name != n.Name {
		return parser.Continue | parser.HasChildren
	}
	// Close only the innermost open component, so that the inner tag of
	// a <Card> nested in a <Card> claims the first </Card>.
	if top, ok := peekOpen(pc); !ok || top != n {
		return parser.Continue | parser.HasChildren
	}
	reader.Advance(seg.Stop - seg.Start - 1)
	n.closed = true
	return parser.Close
}

func (p *blockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	n := node.(*Node)
	if n.SelfClosing {
		return
	}
	if top, ok := peekOpen(pc); ok && top == n {
		popOpen(pc)
	}
	if !n.closed {
		appendError(pc, n.OpenLine, "unclosed <%s>, expected </%s>", n.Name, n.Name)
	}
}

func (p *blockParser) CanInterruptParagraph() bool { return true }
func (p *blockParser) CanAcceptIndentedLine() bool { return false }
