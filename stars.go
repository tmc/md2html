package md2html

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// starCount is the star count shown beside a repository link.
//
// Fetching it is opt-in because it is the one thing on a rendered page
// that requires the network. A site built without it is complete; a site
// built with it is complete too, since a count that cannot be fetched is
// simply not shown.
type starCount struct {
	Stargazers int `json:"stargazers_count"`
}

// githubAPI is the host repository counts are read from. A caller can
// point somewhere else through [Config.starsAPI]; it is a parameter
// rather than a package variable so tests do not have to mutate shared
// state to redirect it, and can therefore run in parallel.
const githubAPI = "https://api.github.com"

// fetchStars reports the star count of an "owner/name" repository.
//
// Anything that goes wrong (no network, a rate limit, a repository that
// is private or gone) reports an error, and the caller renders the link
// without a count rather than failing the build over decoration.
func fetchStars(ctx context.Context, api, repo string) (int, error) {
	if repo == "" {
		return 0, fmt.Errorf("no repository")
	}
	if api == "" {
		api = githubAPI
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := api + "/repos/" + repo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("get %s: %s", url, resp.Status)
	}
	var count starCount
	if err := json.NewDecoder(resp.Body).Decode(&count); err != nil {
		return 0, fmt.Errorf("decode %s: %w", url, err)
	}
	return count.Stargazers, nil
}

// formatStars renders a star count the short way counts are usually
// written, so a wide number does not push the bar around: 993, 1.2k,
// 15k, 1.5m.
func formatStars(n int) string {
	switch {
	case n < 0:
		return ""
	case n < 1000:
		return strconv.Itoa(n)
	case n < 1_000_000:
		return trimPointZero(float64(n)/1000, "k")
	default:
		return trimPointZero(float64(n)/1_000_000, "m")
	}
}

func trimPointZero(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	if v >= 10 {
		s = strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strings.TrimSuffix(s, ".0") + unit
}

// repoStars fetches and formats the star count when the caller asked for
// one. A failure is logged and reported as no count: the repository link
// is still correct without it.
func (s *preparedSite) repoStars(ctx context.Context, repo string, logger *slog.Logger) string {
	if s == nil || !s.config.Stars || repo == "" {
		return ""
	}
	n, err := fetchStars(ctx, s.starsAPI, repo)
	if err != nil {
		if logger != nil {
			logger.Warn("Could not fetch repository stars", "repo", repo, "error", err)
		}
		return ""
	}
	return formatStars(n)
}
