package tabs

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// ParseError describes a parse-time failure with source location.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("tabs: line %d: %s", e.Line, e.Msg)
}

// Context keys.
var (
	slugsContextKey  = parser.NewContextKey()
	errorsContextKey = parser.NewContextKey()
	stackContextKey  = parser.NewContextKey()
)

// openKind tracks what kind of node sits on the open-fence stack.
type openKind int

const (
	openGroup openKind = iota + 1
	openTab
)

type openEntry struct {
	kind openKind
	node ast.Node
}

func pushOpen(pc parser.Context, kind openKind, node ast.Node) {
	v, _ := pc.Get(stackContextKey).([]openEntry)
	v = append(v, openEntry{kind: kind, node: node})
	pc.Set(stackContextKey, v)
}

func popOpen(pc parser.Context) (openEntry, bool) {
	v, _ := pc.Get(stackContextKey).([]openEntry)
	if len(v) == 0 {
		return openEntry{}, false
	}
	top := v[len(v)-1]
	pc.Set(stackContextKey, v[:len(v)-1])
	return top, true
}

func peekOpen(pc parser.Context) (openEntry, bool) {
	v, _ := pc.Get(stackContextKey).([]openEntry)
	if len(v) == 0 {
		return openEntry{}, false
	}
	return v[len(v)-1], true
}

// slugCounts maps groupID -> slug -> count (1-based).
type slugCounts map[string]map[string]int

func getSlugCounts(pc parser.Context) slugCounts {
	v := pc.Get(slugsContextKey)
	if v == nil {
		sc := slugCounts{}
		pc.Set(slugsContextKey, sc)
		return sc
	}
	sc, ok := v.(slugCounts)
	if !ok {
		sc = slugCounts{}
		pc.Set(slugsContextKey, sc)
	}
	return sc
}

// Errors returns any parse errors recorded on pc.
func Errors(pc parser.Context) []*ParseError {
	v := pc.Get(errorsContextKey)
	if v == nil {
		return nil
	}
	errs, _ := v.([]*ParseError)
	return errs
}

func appendError(pc parser.Context, line int, format string, args ...any) {
	errs, _ := pc.Get(errorsContextKey).([]*ParseError)
	errs = append(errs, &ParseError{Line: line, Msg: fmt.Sprintf(format, args...)})
	pc.Set(errorsContextKey, errs)
}

func countFence(line []byte, pos int) (int, bool) {
	i := pos
	for ; i < len(line) && line[i] == ':'; i++ {
	}
	n := i - pos
	return n, n >= 3
}

func isBareClose(line []byte, pos int) bool {
	n, ok := countFence(line, pos)
	if !ok {
		return false
	}
	return util.IsBlank(line[pos+n:])
}

// parseOpeningKeyword scans "<keyword> <rest>" after the fence.
func parseOpeningKeyword(line []byte, pos int) (keyword, rest string, ok bool) {
	rem := bytes.TrimLeft(line[pos:], " \t")
	if len(rem) == 0 {
		return "", "", false
	}
	rem = bytes.TrimRight(rem, " \t\r\n")
	sp := bytes.IndexAny(rem, " \t")
	if sp < 0 {
		return string(rem), "", true
	}
	return string(rem[:sp]), strings.TrimSpace(string(rem[sp:])), true
}

// kebab converts a label to a URL-safe slug.
func kebab(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ', r == '\t', r == '-', r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// uniqueSlug returns a group-scoped slug, disambiguating collisions.
func uniqueSlug(pc parser.Context, groupID, label string) string {
	base := kebab(label)
	if base == "" {
		base = "tab"
	}
	slug := groupID + "-" + base

	counts := getSlugCounts(pc)
	group, ok := counts[groupID]
	if !ok {
		group = map[string]int{}
		counts[groupID] = group
	}
	group[slug]++
	n := group[slug]
	if n == 1 {
		return slug
	}
	return fmt.Sprintf("%s-%d", slug, n)
}

// currentLine returns a 1-based line number for diagnostics.
func currentLine(reader text.Reader) int {
	ln, _ := reader.Position()
	return ln + 1
}

// tabGroupParser recognises "::: tabs <group-id>".
type tabGroupParser struct{}

func (p *tabGroupParser) Trigger() []byte { return []byte{':'} }

func (p *tabGroupParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos >= len(line) || line[pos] != ':' {
		return nil, parser.NoChildren
	}
	fenceLen, ok := countFence(line, pos)
	if !ok {
		return nil, parser.NoChildren
	}

	// Only open at the document/root level. Nested "::: tabs" is
	// rejected by tabParser.Open; it is never accepted here.
	if _, isGroup := parent.(*TabGroup); isGroup {
		return nil, parser.NoChildren
	}
	if _, isTab := parent.(*Tab); isTab {
		return nil, parser.NoChildren
	}

	keyword, rest, hasKW := parseOpeningKeyword(line, pos+fenceLen)
	if !hasKW || keyword != "tabs" {
		return nil, parser.NoChildren
	}
	if rest == "" {
		appendError(pc, currentLine(reader), "::: tabs requires a group id (e.g. ::: tabs install)")
		return nil, parser.NoChildren
	}
	if strings.ContainsAny(rest, " \t") {
		appendError(pc, currentLine(reader), "::: tabs group id must be a single token, got %q", rest)
		return nil, parser.NoChildren
	}

	reader.Advance(len(line) - 1)
	node := NewTabGroup(rest)
	pushOpen(pc, openGroup, node)
	return node, parser.HasChildren
}

func (p *tabGroupParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, seg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return parser.Continue | parser.HasChildren
	}
	// A bare ::: closes the group ONLY when it is the innermost open
	// fence (i.e. no child tab is currently open). When a tab is open,
	// its Continue is called after ours; letting the tab absorb the
	// close is the correct sequencing.
	if pos < len(line) && line[pos] == ':' && isBareClose(line, pos) {
		top, ok := peekOpen(pc)
		if ok && top.kind == openGroup && top.node == node {
			reader.Advance(seg.Stop - seg.Start - 1)
			return parser.Close
		}
	}
	return parser.Continue | parser.HasChildren
}

func (p *tabGroupParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	// Pop our own entry if it is still on top; a dangling tab (missing
	// close fence) should have been popped first by tabParser.Close.
	top, ok := peekOpen(pc)
	if ok && top.kind == openGroup && top.node == node {
		popOpen(pc)
	}
}

func (p *tabGroupParser) CanInterruptParagraph() bool { return true }
func (p *tabGroupParser) CanAcceptIndentedLine() bool { return false }

// tabParser recognises "::: tab <Label>" inside an open TabGroup.
type tabParser struct{}

func (p *tabParser) Trigger() []byte { return []byte{':'} }

func (p *tabParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos >= len(line) || line[pos] != ':' {
		return nil, parser.NoChildren
	}
	fenceLen, ok := countFence(line, pos)
	if !ok {
		return nil, parser.NoChildren
	}
	keyword, rest, hasKW := parseOpeningKeyword(line, pos+fenceLen)
	if !hasKW {
		return nil, parser.NoChildren
	}

	switch keyword {
	case "tab":
		group, ok := parent.(*TabGroup)
		if !ok {
			appendError(pc, currentLine(reader), "::: tab outside of ::: tabs group")
			return nil, parser.NoChildren
		}
		if rest == "" {
			appendError(pc, currentLine(reader), "::: tab requires a label")
			return nil, parser.NoChildren
		}
		slug := uniqueSlug(pc, group.GroupID, rest)
		reader.Advance(len(line) - 1)
		node := NewTab(rest, slug)
		pushOpen(pc, openTab, node)
		return node, parser.HasChildren
	case "tabs":
		if _, ok := parent.(*TabGroup); ok {
			appendError(pc, currentLine(reader), "nested ::: tabs groups are not supported")
			return nil, parser.NoChildren
		}
		if _, ok := parent.(*Tab); ok {
			appendError(pc, currentLine(reader), "nested ::: tabs inside a tab is not supported")
			return nil, parser.NoChildren
		}
		return nil, parser.NoChildren
	}
	return nil, parser.NoChildren
}

func (p *tabParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, seg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return parser.Continue | parser.HasChildren
	}
	if pos < len(line) && line[pos] == ':' {
		// Bare ::: closes this tab.
		if isBareClose(line, pos) {
			reader.Advance(seg.Stop - seg.Start - 1)
			return parser.Close
		}
		// "::: tab ..." starts a new sibling tab — close this one
		// without consuming the line so tabParser.Open can claim it.
		if fenceLen, ok := countFence(line, pos); ok {
			if keyword, _, hasKW := parseOpeningKeyword(line, pos+fenceLen); hasKW {
				if keyword == "tab" {
					return parser.Close
				}
				if keyword == "tabs" {
					appendError(pc, currentLine(reader), "nested ::: tabs inside a tab is not supported")
					return parser.Close
				}
			}
		}
	}
	return parser.Continue | parser.HasChildren
}

func (p *tabParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	top, ok := peekOpen(pc)
	if ok && top.kind == openTab && top.node == node {
		popOpen(pc)
	}
}

func (p *tabParser) CanInterruptParagraph() bool { return true }
func (p *tabParser) CanAcceptIndentedLine() bool { return false }
