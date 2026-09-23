package md2html

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GitVersion represents a git tag or branch that can be used for versioned docs
type GitVersion struct {
	Name   string // Tag or branch name
	Ref    string // Full git reference (e.g., refs/tags/v1.0.0)
	IsTag  bool   // true if this is a tag, false if branch
	Commit string // Short commit hash
}

// GitVersionManager handles git operations for versioned documentation.
//
// It holds the repository location and nothing else. Every git
// subprocess it runs belongs to whatever asked for it (a request, a
// build), so the context comes in with the call rather than being kept
// here. The exported methods, which have no context parameter, use
// [context.Background].
type GitVersionManager struct {
	repoPath string
}

// NewGitVersionManager creates a new git version manager for the given repository
func NewGitVersionManager(repoPath string) *GitVersionManager {
	return &GitVersionManager{repoPath: repoPath}
}

// git returns a git command rooted at the repository.
func (gvm *GitVersionManager) git(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gvm.repoPath
	return cmd
}

// IsGitRepo checks if the given path is inside a git repository
func (gvm *GitVersionManager) IsGitRepo() bool {
	return gvm.isGitRepo(context.Background())
}

func (gvm *GitVersionManager) isGitRepo(ctx context.Context) bool {
	return gvm.git(ctx, "rev-parse", "--git-dir").Run() == nil
}

// ListVersions returns all available versions (tags and optionally branches)
func (gvm *GitVersionManager) ListVersions(includeBranches bool, tagPattern string) ([]GitVersion, error) {
	return gvm.listVersions(context.Background(), includeBranches, tagPattern)
}

func (gvm *GitVersionManager) listVersions(ctx context.Context, includeBranches bool, tagPattern string) ([]GitVersion, error) {
	var versions []GitVersion

	// Get tags
	tags, err := gvm.listTags(ctx, tagPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	versions = append(versions, tags...)

	// Get branches if requested
	if includeBranches {
		branches, err := gvm.listBranches(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list branches: %w", err)
		}
		versions = append(versions, branches...)
	}

	// Sort versions (tags first, then alphabetically)
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].IsTag != versions[j].IsTag {
			return versions[i].IsTag // tags before branches
		}
		return versions[i].Name > versions[j].Name // reverse alphabetical
	})

	return versions, nil
}

// listTags returns all git tags matching the optional pattern
func (gvm *GitVersionManager) listTags(ctx context.Context, pattern string) ([]GitVersion, error) {
	args := []string{"tag", "-l"}
	if pattern != "" {
		args = append(args, pattern)
	}
	args = append(args, "--sort=-version:refname")

	output, err := gvm.git(ctx, args...).Output()
	if err != nil {
		return nil, err
	}

	var versions []GitVersion
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		commit, err := gvm.commitHash(ctx, "refs/tags/"+line)
		if err != nil {
			commit = "unknown"
		}

		versions = append(versions, GitVersion{
			Name:   line,
			Ref:    "refs/tags/" + line,
			IsTag:  true,
			Commit: commit,
		})
	}

	return versions, nil
}

// listBranches returns all git branches
func (gvm *GitVersionManager) listBranches(ctx context.Context) ([]GitVersion, error) {
	output, err := gvm.git(ctx, "branch", "-r", "--format=%(refname:short)").Output()
	if err != nil {
		return nil, err
	}

	var versions []GitVersion
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "HEAD") {
			continue
		}

		// Remove 'origin/' prefix
		branchName := strings.TrimPrefix(line, "origin/")

		commit, err := gvm.commitHash(ctx, line)
		if err != nil {
			commit = "unknown"
		}

		versions = append(versions, GitVersion{
			Name:   branchName,
			Ref:    line,
			IsTag:  false,
			Commit: commit,
		})
	}

	return versions, nil
}

// commitHash returns the short commit hash for a given ref
func (gvm *GitVersionManager) commitHash(ctx context.Context, ref string) (string, error) {
	output, err := gvm.git(ctx, "rev-parse", "--short", ref).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetFileContent returns the content of a file at a specific version
func (gvm *GitVersionManager) GetFileContent(version, filePath string) ([]byte, error) {
	return gvm.fileContent(context.Background(), version, filePath)
}

func (gvm *GitVersionManager) fileContent(ctx context.Context, version, filePath string) ([]byte, error) {
	// Resolve version to a git ref
	ref := version
	if !strings.HasPrefix(ref, "refs/") {
		// Try as tag first
		if _, err := gvm.commitHash(ctx, "refs/tags/"+version); err == nil {
			ref = "refs/tags/" + version
		}
	}

	// Use git show to get file content
	output, err := gvm.git(ctx, "show", ref+":"+filePath).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get file content for %s at %s: %w", filePath, version, err)
	}

	return output, nil
}

// ListFiles returns all files at a specific version matching a pattern
func (gvm *GitVersionManager) ListFiles(version, pattern string) ([]string, error) {
	return gvm.listFiles(context.Background(), version, pattern)
}

func (gvm *GitVersionManager) listFiles(ctx context.Context, version, pattern string) ([]string, error) {
	ref := version
	if !strings.HasPrefix(ref, "refs/") {
		if _, err := gvm.commitHash(ctx, "refs/tags/"+version); err == nil {
			ref = "refs/tags/" + version
		}
	}

	output, err := gvm.git(ctx, "ls-tree", "-r", "--name-only", ref).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list files at %s: %w", version, err)
	}

	var files []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Filter by pattern if provided
		if pattern != "" {
			matched, err := filepath.Match(pattern, filepath.Base(line))
			if err != nil || !matched {
				continue
			}
		}

		files = append(files, line)
	}

	return files, nil
}

// GetCurrentVersion returns the current version (tag or branch)
func (gvm *GitVersionManager) GetCurrentVersion() (string, error) {
	return gvm.currentVersion(context.Background())
}

func (gvm *GitVersionManager) currentVersion(ctx context.Context) (string, error) {
	// Try to get current tag
	if output, err := gvm.git(ctx, "describe", "--tags", "--exact-match").Output(); err == nil {
		return strings.TrimSpace(string(output)), nil
	}

	// Fall back to branch name
	output, err := gvm.git(ctx, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func gitLastUpdated(ctx context.Context, repoPath string, files []markdownFile) (map[string]string, error) {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, filepath.ToSlash(f.RelPath))
	}
	return gitLastUpdatedPaths(ctx, repoPath, paths)
}

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

func gitHasHead(ctx context.Context, repoPath string) bool {
	head := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "HEAD")
	head.Dir = repoPath
	return head.Run() == nil
}

func parseGitLastUpdated(out []byte, want map[string]bool) map[string]string {
	result := make(map[string]string)
	var stamp string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isDigits(line) {
			stamp = line
			continue
		}
		name := filepath.ToSlash(line)
		if stamp == "" || !want[name] || result[name] != "" {
			continue
		}
		sec, err := parseUnix(stamp)
		if err != nil {
			continue
		}
		result[name] = time.Unix(sec, 0).UTC().Format("2006-01-02")
	}
	return result
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func parseUnix(s string) (int64, error) {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid unix time")
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}
