package mdvet

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// GitIgnoredCheck flags links and images whose target exists on disk
// but is excluded by .gitignore.
//
// [LinkCheck] and [AssetsCheck] stat the target and fall silent when it
// is there, which is the wrong answer for a file the repository does
// not carry: it resolves on the author's machine and nowhere else. The
// link is broken for every other checkout, and the local filesystem is
// the one place that cannot say so.
//
// Targets git already tracks are not reported even when a pattern
// matches them, because a tracked file ships regardless. The check is
// silent when git is not installed or the tree is not a repository —
// it has no opinion it can support.
type GitIgnoredCheck struct{}

// Name implements [Check].
func (GitIgnoredCheck) Name() string { return "gitignored" }

// Check implements [Check].
func (GitIgnoredCheck) Check(doc *Document) ([]Diagnostic, error) {
	dir := filepath.Dir(doc.File)

	// Collect first and query once: each distinct repository costs a
	// git invocation, not each link.
	type ref struct {
		dest string
		line int
	}
	targets := make(map[string][]ref)
	var order []string
	add := func(dest string, line int) {
		target, ok := resolveExisting(doc, dir, dest)
		if !ok {
			return
		}
		if _, seen := targets[target]; !seen {
			order = append(order, target)
		}
		targets[target] = append(targets[target], ref{dest: dest, line: line})
	}

	err := ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if dest, ok := nodeDest(n); ok {
			add(dest, lineOf(doc.Source, n))
			return ast.WalkContinue, nil
		}
		// A component names its destinations in attributes, where no
		// link or image node ever appears.
		for _, d := range componentDests(n) {
			add(d.value, d.line)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}

	ignored := doc.env.gitIgnored(order)
	var diags []Diagnostic
	for _, target := range order {
		if !ignored[target] {
			continue
		}
		for _, r := range targets[target] {
			diags = append(diags, Diagnostic{
				File:    doc.File,
				Line:    r.line,
				Check:   "gitignored",
				Message: fmt.Sprintf("link %q: %s is ignored by git; it exists here but not in the repository", r.dest, displayPath(doc.File, target)),
			})
		}
	}
	return diags, nil
}

// resolveExisting turns a link destination into the file it names, and
// reports false for anything this check has no business judging: a
// destination that names no path, an absolute one (LinkCheck's finding),
// and one that resolves to nothing — a missing target is the link and
// asset checks' finding, and reporting it twice helps nobody.
func resolveExisting(doc *Document, dir, dest string) (string, bool) {
	if !shouldCheckOnDisk(dest) {
		return "", false
	}
	var target string
	// A rooted URL the site owns is a page address, so ask the site
	// before judging it an absolute filesystem path.
	if doc.env.site.owns(dest) {
		file, _, ok := doc.env.site.resolve(dest)
		if !ok {
			return "", false
		}
		target = file
	} else {
		if isAbsolutePathLink(dest) {
			return "", false
		}
		file, _, err := resolveLink(dir, dest)
		if err != nil {
			return "", false
		}
		target = file
	}
	if _, err := os.Stat(target); err != nil {
		return "", false
	}
	return target, true
}

// gitIgnorer answers "is this path excluded by git" for the paths the
// checks run into, one git invocation per repository per batch. Both
// answers are cached: a path asked about twice, or by two documents,
// costs nothing the second time.
type gitIgnorer struct {
	ignored map[string]bool   // abs path -> ignored
	roots   map[string]string // dir -> repository root, "" if none
	broken  map[string]bool   // repository roots git could not answer for
	haveGit bool              // git is on PATH
	looked  bool              // haveGit has been determined
}

func newGitIgnorer() *gitIgnorer {
	return &gitIgnorer{
		ignored: make(map[string]bool),
		roots:   make(map[string]string),
		broken:  make(map[string]bool),
	}
}

// gitIgnored reports which of paths git excludes. Paths it cannot
// decide — no git, no repository, a git that failed — are absent from
// the result, which callers read as "not ignored".
func (e *env) gitIgnored(paths []string) map[string]bool {
	return e.git.ignoredSet(paths)
}

func (g *gitIgnorer) ignoredSet(paths []string) map[string]bool {
	abs := make(map[string]string, len(paths)) // path as asked -> absolute
	byRoot := make(map[string][]string)
	for _, p := range paths {
		a, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		abs[p] = a
		if _, ok := g.ignored[a]; ok {
			continue
		}
		root := g.repoRoot(filepath.Dir(a))
		if root == "" || g.broken[root] {
			continue
		}
		byRoot[root] = append(byRoot[root], a)
	}
	if len(byRoot) > 0 && g.available() {
		for root, batch := range byRoot {
			hits, err := checkIgnore(root, batch)
			if err != nil {
				// One failure is enough: a repository git refuses to
				// answer for will refuse again, and a check that keeps
				// spawning a failing subprocess is worse than a silent
				// one.
				g.broken[root] = true
				continue
			}
			for _, a := range batch {
				g.ignored[a] = hits[a]
			}
		}
	}
	out := make(map[string]bool, len(paths))
	for p, a := range abs {
		if got, ok := g.ignored[a]; ok {
			out[p] = got
		}
	}
	return out
}

// available reports whether git is on PATH, looking once.
func (g *gitIgnorer) available() bool {
	if !g.looked {
		g.looked = true
		_, err := exec.LookPath("git")
		g.haveGit = err == nil
	}
	return g.haveGit
}

// repoRoot returns the repository directory governing dir, or "" if dir
// is not in a work tree. It walks up looking for ".git" rather than
// asking git, so a tree with no repository costs no subprocess.
func (g *gitIgnorer) repoRoot(dir string) string {
	if got, ok := g.roots[dir]; ok {
		return got
	}
	root := ""
	seen := []string{}
	for cur := dir; ; {
		if got, ok := g.roots[cur]; ok {
			root = got
			break
		}
		seen = append(seen, cur)
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			root = cur
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	for _, d := range seen {
		g.roots[d] = root
	}
	return root
}

// checkIgnore asks git which of paths it excludes. The paths are given
// relative to root and echoed back verbatim, so the reply maps onto the
// request without further resolution.
func checkIgnore(root string, paths []string) (map[string]bool, error) {
	rels := make(map[string]string, len(paths)) // relative path -> original
	var stdin bytes.Buffer
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		rels[rel] = p
		stdin.WriteString(rel)
		stdin.WriteByte(0)
	}

	cmd := exec.Command("git", "-C", root, "check-ignore", "--stdin", "-z")
	cmd.Stdin = &stdin
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		// Exit status 1 means no path was ignored, which is an answer
		// rather than a failure.
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return nil, fmt.Errorf("git check-ignore: %w", err)
		}
	}

	out := make(map[string]bool, len(paths))
	for _, rel := range strings.Split(stdout.String(), "\x00") {
		if rel == "" {
			continue
		}
		if p, ok := rels[rel]; ok {
			out[p] = true
		}
	}
	return out, nil
}
