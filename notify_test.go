package md2html

import (
	"log/slog"
	"testing"
)

// TestNotifyClientsDelivers verifies that notifyClients actually delivers a
// reload to registered SSE clients. A prior implementation guarded the send
// with an inverted select on time.After, so the default branch was always
// taken and no client was ever notified — live reload silently never fired.
func TestNotifyClientsDelivers(t *testing.T) {
	s := newServer(Config{}, slog.Default())

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
