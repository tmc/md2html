package md2html

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitVersion is a git tag or branch that versioned docs can be served from.
type GitVersion struct {
	Name   string // Tag or branch name
	Ref    string // Full git reference (e.g., refs/tags/v1.0.0)
	IsTag  bool   // true if this is a tag, false if branch
	Commit string // Short commit hash
}

// gitVersions runs the git operations behind versioned documentation.
//
// It holds the repository location and nothing else. Every git
// subprocess it runs belongs to whatever asked for it (a request, a
// build), so the context comes in with the call rather than being kept
// here.
type gitVersions struct {
	repoPath string
}

// newGitVersions returns a gitVersions for the repository at repoPath.
func newGitVersions(repoPath string) *gitVersions {
	return &gitVersions{repoPath: repoPath}
}

// git returns a git command rooted at the repository.
func (g *gitVersions) git(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.repoPath
	return cmd
}

// isRepo reports whether the repository path is inside a git repository.
func (g *gitVersions) isRepo(ctx context.Context) bool {
	return g.git(ctx, "rev-parse", "--git-dir").Run() == nil
}

// versions returns the tags matching tagPattern, followed by the remote
// branches if includeBranches is set. Tags come first, and each group is
// in reverse alphabetical order.
func (g *gitVersions) versions(ctx context.Context, includeBranches bool, tagPattern string) ([]GitVersion, error) {
	vs, err := g.tags(ctx, tagPattern)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	if includeBranches {
		branches, err := g.branches(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing branches: %w", err)
		}
		vs = append(vs, branches...)
	}

	sort.Slice(vs, func(i, j int) bool {
		if vs[i].IsTag != vs[j].IsTag {
			return vs[i].IsTag
		}
		return vs[i].Name > vs[j].Name
	})
	return vs, nil
}

// tags returns the git tags matching the optional pattern.
func (g *gitVersions) tags(ctx context.Context, pattern string) ([]GitVersion, error) {
	args := []string{"tag", "-l"}
	if pattern != "" {
		args = append(args, pattern)
	}
	args = append(args, "--sort=-version:refname")

	output, err := g.git(ctx, args...).Output()
	if err != nil {
		return nil, err
	}

	var vs []GitVersion
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ref := "refs/tags/" + line
		commit, err := g.commitHash(ctx, ref)
		if err != nil {
			commit = "unknown"
		}
		vs = append(vs, GitVersion{
			Name:   line,
			Ref:    ref,
			IsTag:  true,
			Commit: commit,
		})
	}
	return vs, nil
}

// branches returns the remote git branches.
func (g *gitVersions) branches(ctx context.Context) ([]GitVersion, error) {
	output, err := g.git(ctx, "branch", "-r", "--format=%(refname:short)").Output()
	if err != nil {
		return nil, err
	}

	var vs []GitVersion
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "HEAD") {
			continue
		}
		commit, err := g.commitHash(ctx, line)
		if err != nil {
			commit = "unknown"
		}
		vs = append(vs, GitVersion{
			Name:   strings.TrimPrefix(line, "origin/"),
			Ref:    line,
			IsTag:  false,
			Commit: commit,
		})
	}
	return vs, nil
}

// commitHash returns the short commit hash for ref.
func (g *gitVersions) commitHash(ctx context.Context, ref string) (string, error) {
	output, err := g.git(ctx, "rev-parse", "--short", ref).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// resolve returns the git reference for version. A full reference
// (one starting with "refs/") is returned unchanged; otherwise version
// is taken as a tag if such a tag exists, and as a plain revision if not.
func (g *gitVersions) resolve(ctx context.Context, version string) string {
	if strings.HasPrefix(version, "refs/") {
		return version
	}
	if _, err := g.commitHash(ctx, "refs/tags/"+version); err == nil {
		return "refs/tags/" + version
	}
	return version
}

// fileContent returns the content of the file at path as of version.
func (g *gitVersions) fileContent(ctx context.Context, version, path string) ([]byte, error) {
	output, err := g.git(ctx, "show", g.resolve(ctx, version)+":"+path).Output()
	if err != nil {
		return nil, fmt.Errorf("reading %s at %s: %w", path, version, err)
	}
	return output, nil
}

// currentVersion returns the tag at HEAD if there is one, and the
// current branch name otherwise.
func (g *gitVersions) currentVersion(ctx context.Context) (string, error) {
	if output, err := g.git(ctx, "describe", "--tags", "--exact-match").Output(); err == nil {
		return strings.TrimSpace(string(output)), nil
	}

	output, err := g.git(ctx, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("finding current version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// gitLastUpdated returns the date, as YYYY-MM-DD, of the last commit
// touching each of files, keyed by slash-separated relative path.
func gitLastUpdated(ctx context.Context, repoPath string, files []markdownFile) (map[string]string, error) {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, filepath.ToSlash(f.RelPath))
	}
	return gitLastUpdatedPaths(ctx, repoPath, paths)
}

// gitLastUpdatedPaths is like gitLastUpdated but takes paths directly.
// A repository with no commits yields an empty map.
func gitLastUpdatedPaths(ctx context.Context, repoPath string, paths []string) (map[string]string, error) {
	if len(paths) == 0 {
		return map[string]string{}, nil
	}
	if !gitHasHead(ctx, repoPath) {
		return map[string]string{}, nil
	}
	args := []string{"log", "--format=%ct", "--name-only", "--"}
	args = append(args, paths...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool)
	for _, p := range paths {
		want[filepath.ToSlash(p)] = true
	}
	return parseGitLastUpdated(out, want), nil
}

// gitHasHead reports whether the repository at repoPath has a HEAD commit.
func gitHasHead(ctx context.Context, repoPath string) bool {
	head := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "HEAD")
	head.Dir = repoPath
	return head.Run() == nil
}

// parseGitLastUpdated parses the output of git log --format=%ct --name-only,
// newest commit first, and returns the date of the first commit listing
// each wanted path.
func parseGitLastUpdated(out []byte, want map[string]bool) map[string]string {
	result := make(map[string]string)
	var stamp string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if sec, err := strconv.ParseUint(line, 10, 64); err == nil {
			stamp = time.Unix(int64(sec), 0).UTC().Format("2006-01-02")
			continue
		}
		name := filepath.ToSlash(line)
		if stamp == "" || !want[name] || result[name] != "" {
			continue
		}
		result[name] = stamp
	}
	return result
}
