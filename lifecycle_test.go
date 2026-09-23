package md2html

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// freeAddr returns a loopback address that nothing is listening on.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

// waitServing waits for addr to answer an HTTP request. The budget is
// generous because it only bounds a failure: under -race, with the
// script fixtures starting servers of their own, a listener can take
// seconds to come up.
func waitServing(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for {
		resp, err := http.Get("http://" + addr + "/")
		if err == nil {
			resp.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server at %s never answered: %v", addr, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// siteDir writes a one-page source tree and returns its directory.
func siteDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRunInstallsNoSignalHandler checks that Run leaves signal handling
// to its caller. A library that installed its own would decide what ^C
// means for whatever program linked it in; the md2html command installs
// the handler instead.
func TestRunInstallsNoSignalHandler(t *testing.T) {
	dir := siteDir(t)
	addr := freeAddr(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Source: dir, HTTP: addr, Watch: "false"}, discardLogger(), io.Discard, nil)
	}()
	waitServing(t, addr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)
	defer signal.Stop(sig)
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Skipf("cannot signal this process: %v", err)
	}
	select {
	case <-sig:
	case <-time.After(10 * time.Second):
		t.Fatal("SIGINT was not delivered")
	}

	// The interrupt belongs to the caller: the server keeps serving.
	waitServing(t, addr)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() = %v, want nil after cancellation", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after its context was canceled")
	}
}

// fakeGit puts a git on PATH that this test controls, and returns a
// function that replaces its script. Cancellation has to be observed
// against a subprocess that is still running, which no real repository
// offers reliably.
func fakeGit(t *testing.T, script string) func(string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "git")
	write := func(script string) {
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(script)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return write
}

// blockingGit blocks until it is killed, recording that it ran. exec
// replaces the shell, so the process the context kills is this one and
// nothing survives it holding the pipe open.
const blockingGit = `#!/bin/sh
echo "$@" >> "$GIT_PROBE_LOG"
exec sleep 60
`

// workingGit answers the two questions md2html asks of a repository.
const workingGit = `#!/bin/sh
echo "$@" >> "$GIT_PROBE_LOG"
case "$1" in
rev-parse) exit 0 ;;
log) echo 1777939200; echo "page.md"; exit 0 ;;
*) exit 1 ;;
esac
`

// TestGitMetadataCancellationIsNotCached checks that a canceled request
// leaves no answer behind: the probe it abandoned says nothing about the
// repository, and a later live request must still find the date.
func TestGitMetadataCancellationIsNotCached(t *testing.T) {
	dir := siteDir(t)
	log := filepath.Join(t.TempDir(), "probes")
	t.Setenv("GIT_PROBE_LOG", log)
	setGit := fakeGit(t, blockingGit)

	site, err := prepareSite(Config{Source: dir}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(context.Background(), site, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan string, 1)
	go func() { done <- s.lastUpdatedFor(ctx, "page.md") }()

	// Wait for the probe to actually start, then take the request away.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if data, err := os.ReadFile(log); err == nil && len(data) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("git was never invoked")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("lastUpdatedFor() = %q for a canceled request, want empty", got)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("a canceled request did not stop its git subprocess")
	}

	s.lastUpdatedMu.Lock()
	probe, cachedStamp := s.gitMetadata[dir], s.lastUpdated["page.md"]
	s.lastUpdatedMu.Unlock()
	if probe != gitUnknown {
		t.Errorf("git availability for %s cached as %v after cancellation, want unknown", dir, probe)
	}
	if cachedStamp != "" {
		t.Errorf("last-updated cached as %q after cancellation, want nothing", cachedStamp)
	}

	// A live request that follows must succeed and populate both caches.
	setGit(workingGit)
	if got := s.lastUpdatedFor(context.Background(), "page.md"); got != "2026-05-05" {
		t.Fatalf("lastUpdatedFor() = %q after cancellation, want 2026-05-05", got)
	}
	s.lastUpdatedMu.Lock()
	probe, cachedStamp = s.gitMetadata[dir], s.lastUpdated["page.md"]
	s.lastUpdatedMu.Unlock()
	if probe != gitPresent {
		t.Errorf("git availability = %v after a live request, want present", probe)
	}
	if cachedStamp != "2026-05-05" {
		t.Errorf("cached last-updated = %q, want 2026-05-05", cachedStamp)
	}
}

// TestGitMetadataAbsenceIsCached checks that a probe that finished is
// remembered, so a tree with no history is not asked about once a page.
func TestGitMetadataAbsenceIsCached(t *testing.T) {
	dir := siteDir(t)
	log := filepath.Join(t.TempDir(), "probes")
	t.Setenv("GIT_PROBE_LOG", log)
	fakeGit(t, "#!/bin/sh\necho \"$@\" >> \"$GIT_PROBE_LOG\"\nexit 1\n")

	site, err := prepareSite(Config{Source: dir}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(context.Background(), site, discardLogger())

	for range 3 {
		if got := s.lastUpdatedFor(context.Background(), "page.md"); got != "" {
			t.Fatalf("lastUpdatedFor() = %q without git history, want empty", got)
		}
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "\n"); n != 1 {
		t.Errorf("git ran %d times for three requests, want 1:\n%s", n, data)
	}
}

// TestServerRunStopsBackgroundWork checks that cancelling the server's
// context closes the watcher, tells connected browsers to reload, and
// releases every worker Run started.
func TestServerRunStopsBackgroundWork(t *testing.T) {
	dir := siteDir(t)
	addr := freeAddr(t)
	site, err := prepareSite(Config{Source: dir, HTTP: addr, Watch: "true"}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer(ctx, site, discardLogger())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	waitServing(t, addr)

	// Connect to the live-reload stream so shutdown has a client to notify.
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the event stream: %v", err)
	}
	if !strings.Contains(string(body), "data: shutdown") {
		t.Errorf("event stream = %q, want a shutdown message", body)
	}
	if err := s.watcher.Add(dir); err == nil {
		t.Error("the watcher is still open after the server stopped")
	}
}

// TestServerRunListenFailureCleansUp checks that a server that never
// gets to listen still stops the work it had already started.
func TestServerRunListenFailureCleansUp(t *testing.T) {
	dir := siteDir(t)
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()

	site, err := prepareSite(Config{Source: dir, HTTP: taken.Addr().String(), Watch: "true"}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(context.Background(), site, discardLogger())
	if err := s.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded on an address already in use")
	}

	select {
	case <-s.shutdownCh:
	default:
		t.Error("background work was not released after the listener failed")
	}
	if err := s.watcher.Add(dir); err == nil {
		t.Error("the watcher is still open after the listener failed")
	}
}

// TestServerRunInvalidOptionCleansUp checks the same for a configuration
// rejected after watching has already started.
func TestServerRunInvalidOptionCleansUp(t *testing.T) {
	dir := siteDir(t)
	site, err := prepareSite(Config{Source: dir, HTTP: freeAddr(t), Watch: "sometimes"}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(context.Background(), site, discardLogger())
	if err := s.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded with an invalid -watch value")
	}
	select {
	case <-s.shutdownCh:
	default:
		t.Error("background work was not released after an invalid option")
	}
}

// TestRequestsAreIndependent checks that abandoning one request leaves
// the next one unaffected.
func TestRequestsAreIndependent(t *testing.T) {
	dir := siteDir(t)
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Source: dir, HTTP: addr, Watch: "false"}, discardLogger(), io.Discard, nil)
	}()
	waitServing(t, addr)

	abandoned, cancelReq := context.WithCancel(context.Background())
	cancelReq()
	req, err := http.NewRequestWithContext(abandoned, http.MethodGet, "http://"+addr+"/index", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := http.DefaultClient.Do(req); err == nil {
		t.Fatal("a canceled request succeeded")
	}

	resp, err := http.Get("http://" + addr + "/index")
	if err != nil {
		t.Fatalf("the request after a canceled one failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Hi") {
		t.Errorf("page = %q, want the rendered heading", body)
	}

	cancel()
	<-done
}

// TestGitVersions checks the git version operations against a real
// repository.
func TestGitVersions(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# README\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "add readme")
	runGit(t, dir, "tag", "v1.0.0")

	ctx := context.Background()
	g := newGitVersions(dir)
	if !g.isRepo(ctx) {
		t.Fatal("isRepo() = false for a repository")
	}
	versions, err := g.versions(ctx, false, "v*")
	if err != nil {
		t.Fatalf("versions() error = %v", err)
	}
	if len(versions) != 1 || versions[0].Name != "v1.0.0" {
		t.Fatalf("versions() = %#v, want v1.0.0", versions)
	}
	content, err := g.fileContent(ctx, "v1.0.0", "README.md")
	if err != nil {
		t.Fatalf("fileContent() error = %v", err)
	}
	if string(content) != "# README\n" {
		t.Fatalf("fileContent() = %q", content)
	}
	if current, err := g.currentVersion(ctx); err != nil || current != "v1.0.0" {
		t.Fatalf("currentVersion() = %q, %v, want v1.0.0", current, err)
	}
}
