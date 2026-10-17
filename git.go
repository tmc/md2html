package md2html

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GitVersion is a git tag or branch that versioned docs can be served from.
type GitVersion struct {
	Name   string // Tag or branch name
	Ref    string // Full git reference (e.g., refs/tags/v1.0.0)
	IsTag  bool   // true if this is a tag, false if branch
	Commit string // Commit hash
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
