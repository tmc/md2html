package md2html

import (
	"bytes"
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

// GitVersionManager handles git operations for versioned documentation
type GitVersionManager struct {
	repoPath string
}

// NewGitVersionManager creates a new git version manager for the given repository
func NewGitVersionManager(repoPath string) *GitVersionManager {
	return &GitVersionManager{repoPath: repoPath}
}

// IsGitRepo checks if the given path is inside a git repository
func (gvm *GitVersionManager) IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = gvm.repoPath
	return cmd.Run() == nil
}

// ListVersions returns all available versions (tags and optionally branches)
func (gvm *GitVersionManager) ListVersions(includeBranches bool, tagPattern string) ([]GitVersion, error) {
	var versions []GitVersion

	// Get tags
	tags, err := gvm.listTags(tagPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	versions = append(versions, tags...)

	// Get branches if requested
	if includeBranches {
		branches, err := gvm.listBranches()
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
func (gvm *GitVersionManager) listTags(pattern string) ([]GitVersion, error) {
	args := []string{"tag", "-l"}
	if pattern != "" {
		args = append(args, pattern)
	}
	args = append(args, "--sort=-version:refname")

	cmd := exec.Command("git", args...)
	cmd.Dir = gvm.repoPath
	output, err := cmd.Output()
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

		commit, err := gvm.getCommitHash("refs/tags/" + line)
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
func (gvm *GitVersionManager) listBranches() ([]GitVersion, error) {
	cmd := exec.Command("git", "branch", "-r", "--format=%(refname:short)")
	cmd.Dir = gvm.repoPath
	output, err := cmd.Output()
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

		commit, err := gvm.getCommitHash(line)
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

// getCommitHash returns the short commit hash for a given ref
func (gvm *GitVersionManager) getCommitHash(ref string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--short", ref)
	cmd.Dir = gvm.repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetFileContent returns the content of a file at a specific version
func (gvm *GitVersionManager) GetFileContent(version, filePath string) ([]byte, error) {
	// Resolve version to a git ref
	ref := version
	if !strings.HasPrefix(ref, "refs/") {
		// Try as tag first
		if _, err := gvm.getCommitHash("refs/tags/" + version); err == nil {
			ref = "refs/tags/" + version
		}
	}

	// Use git show to get file content
	cmd := exec.Command("git", "show", ref+":"+filePath)
	cmd.Dir = gvm.repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get file content for %s at %s: %w", filePath, version, err)
	}

	return output, nil
}

// ListFiles returns all files at a specific version matching a pattern
func (gvm *GitVersionManager) ListFiles(version, pattern string) ([]string, error) {
	ref := version
	if !strings.HasPrefix(ref, "refs/") {
		if _, err := gvm.getCommitHash("refs/tags/" + version); err == nil {
			ref = "refs/tags/" + version
		}
	}

	cmd := exec.Command("git", "ls-tree", "-r", "--name-only", ref)
	cmd.Dir = gvm.repoPath
	output, err := cmd.Output()
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
	// Try to get current tag
	cmd := exec.Command("git", "describe", "--tags", "--exact-match")
	cmd.Dir = gvm.repoPath
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output)), nil
	}

	// Fall back to branch name
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = gvm.repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func gitLastUpdated(repoPath string, files []markdownFile) (map[string]string, error) {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, filepath.ToSlash(f.RelPath))
	}
	return gitLastUpdatedPaths(repoPath, paths)
}

func gitLastUpdatedPaths(repoPath string, paths []string) (map[string]string, error) {
	if len(paths) == 0 {
		return map[string]string{}, nil
	}
	head := exec.Command("git", "rev-parse", "--verify", "HEAD")
	head.Dir = repoPath
	if err := head.Run(); err != nil {
		return map[string]string{}, nil
	}
	args := []string{"log", "--format=%ct", "--name-only", "--"}
	args = append(args, paths...)
	cmd := exec.Command("git", args...)
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

// CheckoutVersion checks out a specific version (for local development)
func (gvm *GitVersionManager) CheckoutVersion(version string) error {
	cmd := exec.Command("git", "checkout", version)
	cmd.Dir = gvm.repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout %s: %w\n%s", version, err, stderr.String())
	}

	return nil
}
