package docversions

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/tmc/md2html/internal/markdownfile"
)

// A Ref is a git ref published as a version.
type Ref struct {
	Name   string // tag or branch name, as shown to readers
	Ref    string // full reference, such as refs/tags/v1.0.0
	IsTag  bool
	Commit string // full commit hash
}

// A Repo is a git repository holding versioned documentation.
type Repo struct {
	Dir string // any directory inside the repository
}

func (r Repo) git(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("git %s: %s", args[0], msg)
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

// IsRepo reports whether r.Dir is inside a git repository.
func (r Repo) IsRepo(ctx context.Context) bool {
	_, err := r.git(ctx, "rev-parse", "--git-dir")
	return err == nil
}

// Toplevel returns the root directory of the working tree.
func (r Repo) Toplevel(ctx context.Context) (string, error) {
	out, err := r.git(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Commit returns the full commit hash rev names.
func (r Repo) Commit(ctx context.Context, rev string) (string, error) {
	out, err := r.git(ctx, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", rev, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Tags returns the tags matching the glob pattern, newest first by
// [Compare]. An empty pattern matches every tag.
func (r Repo) Tags(ctx context.Context, pattern string) ([]Ref, error) {
	// refname:short would say tags/v2 for a tag v2 with a branch v2.
	args := []string{"for-each-ref", "--format=%(refname:lstrip=2)%00%(objectname)%00%(*objectname)", "refs/tags/"}
	out, err := r.git(ctx, args...)
	if err != nil {
		return nil, err
	}
	var refs []Ref
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		name, rest, ok := strings.Cut(line, "\x00")
		if !ok {
			continue
		}
		if pattern != "" && pattern != "*" {
			if ok, _ := path.Match(pattern, name); !ok {
				continue
			}
		}
		commit, peeled, _ := strings.Cut(rest, "\x00")
		if peeled != "" {
			commit = peeled // an annotated tag names a tag object
		}
		refs = append(refs, Ref{Name: name, Ref: "refs/tags/" + name, IsTag: true, Commit: commit})
	}
	sortRefs(refs)
	return refs, nil
}

// Branches returns the remote-tracking branches, named without their
// remote, in reverse alphabetical order.
func (r Repo) Branches(ctx context.Context) ([]Ref, error) {
	out, err := r.git(ctx, "for-each-ref", "--format=%(refname:lstrip=2)%00%(refname)%00%(objectname)", "refs/remotes/")
	if err != nil {
		return nil, err
	}
	var refs []Ref
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 || strings.HasSuffix(fields[0], "/HEAD") || !strings.Contains(fields[0], "/") {
			continue
		}
		_, name, _ := strings.Cut(fields[0], "/")
		refs = append(refs, Ref{Name: name, Ref: fields[1], Commit: fields[2]})
	}
	sortRefs(refs)
	return refs, nil
}

func sortRefs(refs []Ref) {
	names := make(map[string]Ref, len(refs))
	keys := make([]string, 0, len(refs))
	for _, r := range refs {
		names[r.Name] = r
		keys = append(keys, r.Name)
	}
	SortNewestFirst(keys)
	for i, k := range keys {
		refs[i] = names[k]
	}
}

// AddWorktree checks commit out, detached, into the new directory dir.
func (r Repo) AddWorktree(ctx context.Context, dir, commit string) error {
	_, err := r.git(ctx, "worktree", "add", "--quiet", "--detach", dir, commit)
	return err
}

// RemoveWorktree removes a worktree made by AddWorktree.
func (r Repo) RemoveWorktree(ctx context.Context, dir string) error {
	_, err := r.git(ctx, "worktree", "remove", "--force", dir)
	return err
}

// Pages returns the Markdown pages under dir, a slash-separated path
// relative to the repository root, as of commit. Paths are relative to
// dir.
func (r Repo) Pages(ctx context.Context, commit, dir string) ([]string, error) {
	// -z prints names as they are, not quoted as core.quotePath has
	// git do for names outside ASCII.
	args := []string{"ls-tree", "-r", "-z", "--name-only", "--full-tree", commit}
	if dir != "" && dir != "." {
		args = append(args, "--", dir+"/")
	}
	out, err := r.git(ctx, args...)
	if err != nil {
		return nil, err
	}
	var pages []string
	for name := range strings.SplitSeq(string(out), "\x00") {
		if rel, ok := under(dir, name); ok && markdownfile.Is(rel) {
			pages = append(pages, rel)
		}
	}
	return pages, nil
}

// Renames returns the Markdown pages under dir that git's rename
// detection finds moved between commits from and to, mapping each old
// path to its new one. An empty to compares against the working tree.
// Paths are relative to dir.
func (r Repo) Renames(ctx context.Context, from, to, dir string) (map[string]string, error) {
	renamed, _, err := r.Changes(ctx, from, to, dir)
	return renamed, err
}

// Changes is like [Repo.Renames] but also returns the Markdown pages
// deleted between from and to.
func (r Repo) Changes(ctx context.Context, from, to, dir string) (renamed map[string]string, deleted map[string]bool, err error) {
	args := []string{"diff", "-M", "-z", "--name-status", "--no-color", from}
	if to != "" {
		args = append(args, to)
	}
	if dir != "" && dir != "." {
		args = append(args, "--", ":(top)"+dir+"/")
	}
	out, err := r.git(ctx, args...)
	if err != nil {
		return nil, nil, err
	}
	renamed, deleted = parseChanges(out, dir)
	return renamed, deleted, nil
}

// parseChanges parses git diff -z --name-status output into the
// renamed and the deleted Markdown pages under dir. Each entry is a
// status followed by one path, or by two for a rename or copy, all
// NUL-terminated.
func parseChanges(out []byte, dir string) (map[string]string, map[string]bool) {
	moves := make(map[string]string)
	deleted := make(map[string]bool)
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); {
		status := fields[i]
		if status == "" {
			break
		}
		if status[0] != 'R' && status[0] != 'C' {
			if i+1 < len(fields) && status[0] == 'D' {
				if p, ok := under(dir, fields[i+1]); ok && markdownfile.Is(p) {
					deleted[p] = true
				}
			}
			i += 2
			continue
		}
		if i+2 >= len(fields) {
			break
		}
		oldPath, ok1 := under(dir, fields[i+1])
		newPath, ok2 := under(dir, fields[i+2])
		if status[0] == 'R' && ok1 && ok2 && markdownfile.Is(oldPath) && markdownfile.Is(newPath) {
			moves[oldPath] = newPath
		}
		i += 3
	}
	return moves, deleted
}

// under returns name relative to dir, if it is inside it.
func under(dir, name string) (string, bool) {
	if dir == "" || dir == "." {
		return name, name != ""
	}
	rel, ok := strings.CutPrefix(name, dir+"/")
	return rel, ok && rel != ""
}

// RelPath returns dir relative to top, slash-separated, resolving
// symbolic links in both so that a repository reached through one is
// still found to contain dir. It is an error for dir to be outside top.
func RelPath(top, dir string) (string, error) {
	if t, err := filepath.EvalSymlinks(top); err == nil {
		top = t
	}
	if d, err := filepath.EvalSymlinks(dir); err == nil {
		dir = d
	}
	rel, err := filepath.Rel(top, dir)
	if err != nil || rel != "." && !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%s is outside the repository at %s", dir, top)
	}
	return filepath.ToSlash(rel), nil
}
