package md2html

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestFormatStars(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{4, "4"},
		{999, "999"},
		{1000, "1k"},
		{1234, "1.2k"},
		{9950, "9.9k"},
		{15000, "15k"},
		{999999, "1000k"},
		{1_500_000, "1.5m"},
		{-1, ""},
	}
	for _, tt := range tests {
		if got := formatStars(tt.in); got != tt.want {
			t.Errorf("formatStars(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFetchStars(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/tmc/cdp":
			json.NewEncoder(w).Encode(map[string]any{"stargazers_count": 42})
		case "/repos/tmc/private":
			http.Error(w, "not found", http.StatusNotFound)
		default:
			w.Write([]byte("not json"))
		}
	}))
	defer srv.Close()

	if got, err := fetchStars(context.Background(), srv.URL, "tmc/cdp"); err != nil || got != 42 {
		t.Errorf("fetchStars() = %d, %v, want 42, nil", got, err)
	}
	// A repository that cannot be read is an error, never a zero count
	// rendered as if it were real.
	for _, repo := range []string{"tmc/private", "tmc/garbage", ""} {
		if _, err := fetchStars(context.Background(), srv.URL, repo); err == nil {
			t.Errorf("fetchStars(%q) succeeded, want an error", repo)
		}
	}
}

// TestRepoStarsOptIn checks that no request is made unless asked for,
// and that a failure leaves the link without a count rather than
// failing the render.
func TestRepoStarsOptIn(t *testing.T) {
	t.Parallel()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		json.NewEncoder(w).Encode(map[string]any{"stargazers_count": 7})
	}))
	defer srv.Close()
	logger := discardLogger()
	stars := Config{Stars: true, starsAPI: srv.URL}
	if got := repoStars(context.Background(), Config{starsAPI: srv.URL}, "tmc/cdp", logger); got != "" {
		t.Errorf("repoStars() = %q without -github-stars, want empty", got)
	}
	if hits != 0 {
		t.Errorf("made %d requests without -github-stars, want 0", hits)
	}
	if got := repoStars(context.Background(), stars, "", logger); got != "" {
		t.Errorf("repoStars() = %q with no repository, want empty", got)
	}
	if got := repoStars(context.Background(), stars, "tmc/cdp", logger); got != "7" {
		t.Errorf("repoStars() = %q, want %q", got, "7")
	}
}
