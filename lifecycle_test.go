package md2html

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

// waitServing waits for addr to answer an HTTP request.
func waitServing(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
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
