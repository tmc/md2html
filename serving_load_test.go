package md2html

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestServesLargeFile checks that a document far larger than anything in
// the script fixtures is rendered whole. The tail of the file is what
// matters: a renderer that truncates or times out still answers 200 with
// the first few hundred lines.
func TestServesLargeFile(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	const lines = 10000
	for i := range lines {
		fmt.Fprintf(&b, "Line %d: This is a test line with some **bold** text and *italic* text.\n\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Source: dir, HTTP: addr, Watch: "false"}, discardLogger(), io.Discard, nil)
	}()
	waitServing(t, addr)

	resp, err := http.Get("http://" + addr + "/index")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("Line %d:", lines-1); !strings.Contains(string(body), want) {
		t.Errorf("page is missing %q; the document was truncated", want)
	}

	cancel()
	<-done
}

// TestConcurrentRequests checks that simultaneous requests are all served.
// Rendering caches and the file watcher share state across requests, so a
// missing lock shows up here as a failed request or a data race under
// -race, not in the sequential tests.
func TestConcurrentRequests(t *testing.T) {
	dir := siteDir(t)
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Source: dir, HTTP: addr, Watch: "false"}, discardLogger(), io.Discard, nil)
	}()
	waitServing(t, addr)

	const requests = 50
	var wg sync.WaitGroup
	codes := make([]int, requests)
	errs := make([]error, requests)
	for i := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get("http://" + addr + "/index")
			if err != nil {
				errs[i] = err
				return
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			codes[i] = resp.StatusCode
		}()
	}
	wg.Wait()

	for i := range requests {
		if errs[i] != nil {
			t.Errorf("request %d: %v", i, errs[i])
			continue
		}
		if codes[i] != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i, codes[i], http.StatusOK)
		}
	}

	cancel()
	<-done
}
