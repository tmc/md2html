// Package include expands Code tags, which pull a file into a page as a
// fenced code block.
//
// A page shows a program by naming it:
//
//	<Code src="../examples/hello/main.go" />
//
// which renders as if the author had pasted the file into a fence, so
// the page cannot drift from code that is compiled and tested. The
// attributes are
//
//	src     path to the file (required)
//	lang    fence language; by default taken from the file name
//	region  show only the lines between "region NAME" and "endregion NAME" comments
//	        (the comments themselves are never shown)
//	title   caption for the block
//	link    link the caption to the file
//
// A relative src is resolved against the directory of the page, and a
// src beginning with a slash against the include root. Either way the
// file must be inside the root. Expansion works on Markdown source, so
// everything downstream of it, from the highlighter to llms.txt, sees
// an ordinary fence.
package include

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxSize is the largest file a page may include. A page is not the
// place for anything bigger.
const maxSize = 256 << 10

// A Resolver expands the Code tags of the pages of one site.
type Resolver struct {
	// Root is the directory included files must be inside.
	Root string

	// Excluded, if not nil, reports whether the site keeps the file at
	// the given absolute path out of what it publishes.
	Excluded func(path string) bool

	// Link, if not nil, returns the address of the file at the given
	// slash-separated path relative to Root. When first is not zero the
	// address is of lines first through last. A tag that asks for a
	// link is an error when Link is nil or returns "".
	Link func(path string, first, last int) string
}

// An Error describes a Code tag that could not be expanded.
type Error struct {
	Line int // line of the tag in the page, counting from 1
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

// HasTag reports whether src may contain a Code tag. It is a cheap test
// for callers that would rather not set up a Resolver for nothing.
func HasTag(src []byte) bool {
	return bytes.Contains(src, []byte("<Code"))
}

// Expand returns src, the Markdown source of a page in the directory
// dir, with each Code tag replaced by the fenced code block it names.
// It also returns the absolute paths of the files it read, so that a
// caller can watch them for changes.
//
// A tag that cannot be expanded is replaced by a caution alert saying
// why, and described in errs. A nil Resolver, or an empty dir, expands
// nothing and reports every tag as an error.
func (r *Resolver) Expand(dir string, src []byte) (out []byte, files []string, errs []*Error) {
	if !HasTag(src) {
		return src, nil, nil
	}
	var buf bytes.Buffer
	var fence fenceState
	lines := bytes.SplitAfter(src, []byte("\n"))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if fence.update(line) || !isTagStart(line) {
			buf.Write(line)
			continue
		}
		// A tag may be broken over several lines.
		start := i
		tag := string(line)
		for !strings.Contains(tag, ">") && i+1 < len(lines) {
			i++
			tag += string(lines[i])
		}
		indent := tag[:len(tag)-len(strings.TrimLeft(tag, " \t"))]
		block, file, err := r.expandTag(dir, tag)
		if err != nil {
			errs = append(errs, &Error{Line: start + 1, Msg: err.Error()})
			block = "> [!CAUTION]\n> Code: " + err.Error() + "\n"
		}
		if file != "" {
			files = append(files, file)
		}
		for l := range strings.Lines(block) {
			buf.WriteString(indent)
			buf.WriteString(l)
		}
	}
	return buf.Bytes(), files, errs
}

// isTagStart reports whether line begins a Code tag.
func isTagStart(line []byte) bool {
	rest, ok := bytes.CutPrefix(bytes.TrimLeft(line, " \t"), []byte("<Code"))
	if !ok || len(rest) == 0 {
		return false
	}
	switch rest[0] {
	case ' ', '\t', '\n', '\r', '/', '>':
		return true
	}
	return false
}

// A fenceState tracks whether the lines of a page are inside a fenced
// code block, where a Code tag is an example and not an instruction.
type fenceState struct {
	marker byte
	n      int
}

// update accounts for line and reports whether it is part of a fence.
func (f *fenceState) update(line []byte) bool {
	trimmed := bytes.TrimLeft(line, " ")
	var marker byte
	n := 0
	if len(trimmed) > 0 && (trimmed[0] == '`' || trimmed[0] == '~') {
		marker = trimmed[0]
		for n < len(trimmed) && trimmed[n] == marker {
			n++
		}
	}
	if f.n == 0 {
		if n >= 3 {
			f.marker, f.n = marker, n
			return true
		}
		return false
	}
	if marker == f.marker && n >= f.n && len(bytes.TrimSpace(trimmed[n:])) == 0 {
		f.n = 0
	}
	return true
}

// expandTag returns the Markdown that replaces tag and the absolute
// path of the file it read. The path is returned even with an error
// when the tag named a file, so that fixing the file redraws the page.
func (r *Resolver) expandTag(dir, tag string) (block, file string, err error) {
	attrs, err := parseTag(tag)
	if err != nil {
		return "", "", err
	}
	if r == nil || r.Root == "" || dir == "" {
		return "", "", fmt.Errorf("files cannot be included here")
	}
	src := attrs["src"]
	if src == "" {
		return "", "", fmt.Errorf("missing src")
	}
	rel, err := r.rel(dir, src)
	if err != nil {
		return "", "", err
	}
	file = filepath.Join(r.Root, filepath.FromSlash(rel))
	data, err := r.read(rel)
	if err != nil {
		return "", file, err
	}

	first, last := 0, 0
	if name, ok := attrs["region"]; ok {
		data, first, last, err = region(data, name)
		if err != nil {
			return "", file, fmt.Errorf("%s: %w", src, err)
		}
	} else {
		data = withoutMarkers(data)
	}

	lang, ok := attrs["lang"]
	if !ok {
		lang = language(rel)
	}
	title := attrs["title"]
	href := ""
	if link := attrs["link"]; link == "true" {
		if r.Link != nil {
			href = r.Link(rel, first, last)
		}
		if href == "" {
			return "", file, fmt.Errorf("link needs a repository: name one in docs.json")
		}
		if title == "" {
			title = rel
		}
	} else if link != "" && link != "false" {
		return "", file, fmt.Errorf("link must be true or false")
	}

	var info strings.Builder
	info.WriteString(lang)
	if title != "" || href != "" {
		info.WriteString(" {")
		if title != "" {
			fmt.Fprintf(&info, "title=%s", quote(title))
		}
		if href != "" {
			fmt.Fprintf(&info, " href=%s", quote(href))
		}
		info.WriteString("}")
	}
	// A backtick fence cannot have a backtick in its info string, so a
	// title with one is fenced with tildes.
	c := byte('`')
	if strings.Contains(info.String(), "`") {
		c = '~'
	}
	fence := strings.Repeat(string(c), max(3, longestRun(data, c)+1))
	var b strings.Builder
	b.WriteString(fence)
	b.WriteString(info.String())
	b.WriteString("\n")
	b.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString(fence)
	b.WriteString("\n")
	return b.String(), file, nil
}

// quote returns s as a double-quoted fence attribute value. Quotes and
// backslashes cannot be written in one, and a newline would end the
// fence line, so they are replaced.
func quote(s string) string {
	s = strings.NewReplacer(`"`, "'", `\`, "/", "\n", " ", "\r", " ").Replace(s)
	return `"` + s + `"`
}

func longestRun(data []byte, c byte) int {
	longest, n := 0, 0
	for _, b := range data {
		if b != c {
			n = 0
			continue
		}
		n++
		longest = max(longest, n)
	}
	return longest
}

// rel returns the slash-separated path, relative to the root, of the
// file src names in a page in dir.
func (r *Resolver) rel(dir, src string) (string, error) {
	if strings.Contains(src, "://") || strings.Contains(src, `\`) {
		return "", fmt.Errorf("%s: not a file path", src)
	}
	var name string
	if strings.HasPrefix(src, "/") {
		name = path.Clean(src)[1:]
	} else {
		pageDir, err := filepath.Rel(r.Root, dir)
		if err != nil {
			return "", fmt.Errorf("%s: %w", src, err)
		}
		name = path.Join(filepath.ToSlash(pageDir), src)
	}
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("%s: outside the include root", src)
	}
	for elem := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(elem, ".") {
			return "", fmt.Errorf("%s: hidden files cannot be included", src)
		}
	}
	return name, nil
}

// read returns the contents of the file at rel, a path relative to the
// root. The file is opened through an [os.Root], so no symbolic link
// can lead it outside.
func (r *Resolver) read(rel string) ([]byte, error) {
	name := filepath.FromSlash(rel)
	if r.Excluded != nil && r.Excluded(filepath.Join(r.Root, name)) {
		return nil, fmt.Errorf("%s: excluded by the ignore file", rel)
	}
	root, err := os.OpenRoot(r.Root)
	if err != nil {
		return nil, fmt.Errorf("include root: %w", err)
	}
	defer root.Close()
	info, err := root.Stat(name)
	if err != nil {
		return nil, cleanError(rel, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", rel)
	}
	if info.Size() > maxSize {
		return nil, fmt.Errorf("%s: larger than %d KiB", rel, maxSize>>10)
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return nil, cleanError(rel, err)
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return nil, fmt.Errorf("%s: not a text file", rel)
	}
	return data, nil
}

// cleanError describes err, from an operation on rel, without the
// operation and absolute path an [os.PathError] carries: the message
// is shown in the page.
func cleanError(rel string, err error) error {
	if os.IsNotExist(err) {
		return fmt.Errorf("%s: no such file", rel)
	}
	if pe, ok := err.(*os.PathError); ok {
		err = pe.Err
	}
	return fmt.Errorf("%s: %w", rel, err)
}

var languages = map[string]string{
	".bash": "bash",
	".cc":   "cpp",
	".h":    "c",
	".hpp":  "cpp",
	".htm":  "html",
	".js":   "javascript",
	".md":   "markdown",
	".mod":  "go.mod",
	".py":   "python",
	".rb":   "ruby",
	".rs":   "rust",
	".ts":   "typescript",
	".txt":  "text",
	".yml":  "yaml",
}

// language returns the fence language for the file called name: the
// usual name where the extension is not it, and otherwise the
// extension. A file without an extension is named for itself, which
// suits a Makefile or a Dockerfile.
func language(name string) string {
	base := strings.ToLower(path.Base(filepath.ToSlash(name)))
	ext := path.Ext(base)
	if ext == "" || ext == base {
		return strings.TrimPrefix(base, ".")
	}
	if lang, ok := languages[ext]; ok {
		return lang
	}
	return ext[1:]
}

var attrPattern = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9_-]*)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|\{\s*(true|false)\s*\}|([^\s"'/>{}]+)))?`)

// parseTag returns the attributes of a Code tag. An attribute written
// without a value is "true".
func parseTag(tag string) (map[string]string, error) {
	rest := strings.TrimPrefix(strings.TrimSpace(tag), "<Code")
	attrs := make(map[string]string)
	for {
		rest = strings.TrimSpace(rest)
		switch {
		case rest == "/>":
			return attrs, nil
		case rest == "":
			return nil, fmt.Errorf("unterminated tag")
		case strings.HasPrefix(rest, ">"), strings.HasPrefix(rest, "/>"):
			return nil, fmt.Errorf(`write the tag on its own as <Code src="..." />`)
		}
		m := attrPattern.FindStringSubmatchIndex(rest)
		if m == nil {
			return nil, fmt.Errorf("malformed attribute near %q", firstWord(rest))
		}
		name := rest[m[2]:m[3]]
		value := "true"
		for g := 2; g <= 5; g++ {
			if m[2*g] >= 0 {
				value = rest[m[2*g]:m[2*g+1]]
			}
		}
		switch name {
		case "src", "lang", "region", "title", "link":
		default:
			return nil, fmt.Errorf("unknown attribute %s", name)
		}
		if _, dup := attrs[name]; dup {
			return nil, fmt.Errorf("duplicate attribute %s", name)
		}
		attrs[name] = value
		rest = rest[m[1]:]
	}
}

func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// regionMarker matches a comment marking a region: "region", or
// "endregion", and at most one word naming it. A comment that goes on
// to say more, such as "// region of memory to scan", is prose.
var regionMarker = regexp.MustCompile(`^\s*(?://|#|--|;|/\*|<!--)\s*#?(end)?region(?:[ \t]+(\S+?))?\s*(?:\*/|-->)?\s*$`)

// region returns the lines of data between the comments "region name"
// and "endregion name", without lines that mark other regions and
// without the indentation the lines share. It also returns the numbers
// of the first and last lines of the region, counting from 1.
func region(data []byte, name string) (out []byte, first, last int, err error) {
	if name == "" {
		return nil, 0, 0, fmt.Errorf("empty region name")
	}
	var lines []string
	inside := false
	depth := 0 // regions begun inside this one and not yet ended
	n := 0
	for line := range strings.Lines(string(data)) {
		n++
		m := regionMarker.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
		if m == nil {
			if inside {
				lines = append(lines, line)
				last = n
			}
			continue
		}
		end, marked := m[1] != "", m[2]
		switch {
		case !end && marked == name:
			if inside || first != 0 {
				return nil, 0, 0, fmt.Errorf("region %s begins twice", name)
			}
			inside = true
			first = n + 1
		case !end && inside:
			depth++
		case end && inside && (marked == name || marked == "" && depth == 0):
			inside = false
		case end && inside && depth > 0:
			depth--
		}
	}
	switch {
	case first == 0:
		return nil, 0, 0, fmt.Errorf("no region %s", name)
	case inside:
		return nil, 0, 0, fmt.Errorf("region %s does not end", name)
	case len(lines) == 0:
		return nil, 0, 0, fmt.Errorf("region %s is empty", name)
	}
	return []byte(dedent(lines)), first, last, nil
}

// withoutMarkers returns data without the comments that mark regions,
// which mean nothing to the reader of a whole file.
func withoutMarkers(data []byte) []byte {
	if !bytes.Contains(data, []byte("region")) {
		return data
	}
	var b strings.Builder
	for line := range strings.Lines(string(data)) {
		if !regionMarker.MatchString(strings.TrimRight(line, "\r\n")) {
			b.WriteString(line)
		}
	}
	return []byte(b.String())
}

// dedent joins lines after removing the leading white space that every
// line with text has in common.
func dedent(lines []string) string {
	prefix := ""
	seen := false
	for _, line := range lines {
		text := strings.TrimLeft(line, " \t")
		if strings.TrimSpace(text) == "" {
			continue
		}
		indent := line[:len(line)-len(text)]
		if !seen {
			prefix, seen = indent, true
			continue
		}
		for !strings.HasPrefix(indent, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	var b strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			b.WriteString(strings.TrimLeft(line, " \t"))
			continue
		}
		b.WriteString(strings.TrimPrefix(line, prefix))
	}
	return b.String()
}
