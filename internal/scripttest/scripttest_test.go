package scripttest

import (
	"context"
	"net"
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestRewriteScriptPorts(t *testing.T) {
	in := `md2html -http :8097 code.md &
curl -s http://localhost:8097/
curl -s http://127.0.0.1:8097/raw
md2html -http :8098 other.md &
curl -s http://localhost:8098/
`

	out, err := rewriteScriptPorts(in)
	if err != nil {
		t.Fatalf("rewriteScriptPorts() error = %v", err)
	}

	if out == in {
		t.Fatal("rewriteScriptPorts() did not change any ports")
	}

	rePort := regexp.MustCompile(`\b\d{4,5}\b`)
	ports := rePort.FindAllString(out, -1)
	if len(ports) < 5 {
		t.Fatalf("expected rewritten ports in output, got %q", out)
	}

	if ports[0] != ports[1] || ports[0] != ports[2] {
		t.Fatalf("same original port should rewrite consistently, got %v", ports[:3])
	}
	if ports[3] != ports[4] {
		t.Fatalf("same original port should rewrite consistently, got %v", ports[3:5])
	}
	if ports[0] == ports[3] {
		t.Fatalf("distinct original ports should rewrite to distinct free ports, got %v", ports)
	}
}

func TestRewriteScriptPortsRange(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		rewritten []string
		kept      []string
	}{
		{
			name:      "nine thousand port",
			in:        "md2html -http :9081 doc.md\ncurl http://localhost:9081/\ncurl http://127.0.0.1:9081/\n",
			rewritten: []string{":9081", "localhost:9081", "127.0.0.1:9081"},
		},
		{
			name:      "eight thousand port",
			in:        "md2html -http :8765 doc.md\n",
			rewritten: []string{":8765"},
		},
		{
			name:      "five digit port",
			in:        "md2html -http :12345 doc.md\ncurl http://localhost:12345/\n",
			rewritten: []string{":12345", "localhost:12345"},
		},
		{
			name: "short port ignored",
			in:   "md2html -http :99 doc.md\ncurl http://localhost:99/\n",
			kept: []string{":99", "localhost:99"},
		},
		{
			name: "larger numbers ignored",
			in:   "echo 90810\necho 81234567\n",
			kept: []string{"90810", "81234567"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := rewriteScriptPorts(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range tt.rewritten {
				if regexp.MustCompile(regexp.QuoteMeta(s)).MatchString(out) {
					t.Fatalf("%q was not rewritten:\n%s", s, out)
				}
			}
			for _, s := range tt.kept {
				if !regexp.MustCompile(regexp.QuoteMeta(s)).MatchString(out) {
					t.Fatalf("%q was unexpectedly rewritten:\n%s", s, out)
				}
			}
		})
	}
}

func TestWaitPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	err = waitPort(context.Background(), ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("waitPort() error = %v", err)
	}
}

func TestReserveTestPortUnique(t *testing.T) {
	const count = 100
	ports := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			port, err := reserveTestPort()
			ports <- port
			errs <- err
		}()
	}
	wg.Wait()
	close(ports)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[string]bool)
	for port := range ports {
		if seen[port] {
			t.Fatalf("reserveTestPort() returned duplicate port %s", port)
		}
		seen[port] = true
	}
}
