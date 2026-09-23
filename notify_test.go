package md2html

import (
	"context"
	"log/slog"
	"testing"
)

// TestNotifyClientsDelivers checks that notifyClients delivers a reload
// to every registered SSE client. A send that never happens fails
// silently in the browser: the page just stops reloading.
func TestNotifyClientsDelivers(t *testing.T) {
	s := newServer(context.Background(), &preparedSite{}, slog.Default())

	ch := make(chan string, 1)
	s.clientsMu.Lock()
	s.clients[ch] = true
	s.clientsMu.Unlock()

	s.notifyClients()

	select {
	case msg := <-ch:
		if msg != "reload" {
			t.Fatalf("got %q, want \"reload\"", msg)
		}
	default:
		t.Fatal("notifyClients did not deliver a reload to the registered client")
	}
}
