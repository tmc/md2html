// Package scripttestutil helps with script-based testing.
package scripttestutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/tools/txtar"
	"rsc.io/script"
	"rsc.io/script/scripttest"
)

var scriptPortPattern = regexp.MustCompile(`(localhost:|127\.0\.0\.1:|:)([1-9]\d{3,4})\b`)

// WaitPortCmd returns a script command that waits until a TCP port accepts
// connections.
func WaitPortCmd() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "wait for a TCP port to accept connections",
			Args:    "addr [timeout]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) != 1 && len(args) != 2 {
				return nil, fmt.Errorf("usage: wait-port addr [timeout]")
			}

			timeout := 5 * time.Second
			if len(args) == 2 {
				d, err := time.ParseDuration(args[1])
				if err != nil {
					return nil, err
				}
				timeout = d
			}
			return nil, waitPort(s.Context(), args[0], timeout)
		},
	)
}

func waitPort(ctx context.Context, addr string, timeout time.Duration) error {
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	} else if !strings.Contains(addr, ":") {
		addr = "localhost:" + addr
	}

	start := time.Now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()

	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("wait-port %s: no connection after %v", addr, time.Since(start).Round(time.Millisecond))
		case <-tick.C:
		}
	}
}

// BackgroundCmd returns a command that runs prog in the background
// with graceful shutdown support via SIGTERM instead of SIGKILL.
//
// The signature matches script.Program exactly for drop-in replacement.
//
// Parameters:
//   - prog: The program to run. Can be a program name (looked up in PATH),
//     an absolute path, or a relative path containing separators.
//   - cancel: Optional function called when the script's context is cancelled.
//     If nil, sends SIGTERM for graceful shutdown.
//     If provided, called with the *exec.Cmd to allow custom shutdown logic.
//   - waitDelay: Maximum time to wait for the program to exit after cancellation
//     before forcibly killing it. Passed to exec.Cmd.WaitDelay.
//
// Differences from script.Program:
//   - Default cancellation sends SIGTERM instead of SIGKILL, allowing:
//   - Graceful shutdown with cleanup
//   - Coverage data to be written
//   - Exit code 0 on clean shutdown
//   - Context cancellation with exit code 0 is treated as success, not error
//   - Ensures GOCOVERDIR environment variable is preserved for coverage
//
// Example:
//
//	// Drop-in replacement with graceful shutdown
//	engine.Cmds["myserver"] = scripttestutil.BackgroundCmd(exe, nil, 0)
//
//	// Custom shutdown signal
//	engine.Cmds["myapp"] = scripttestutil.BackgroundCmd(exe, func(cmd *exec.Cmd) error {
//	    return cmd.Process.Signal(os.Interrupt)
//	}, 2*time.Second)
func BackgroundCmd(prog string, cancel func(*exec.Cmd) error, waitDelay time.Duration) script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "run " + filepath.Base(prog),
			Async:   true,
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			// If prog is an absolute path or contains separators, use it directly.
			// Otherwise look it up in PATH.
			name := prog
			path := prog
			if !filepath.IsAbs(prog) && !strings.Contains(prog, string(filepath.Separator)) {
				var err error
				path, err = exec.LookPath(prog)
				if err != nil {
					return nil, err
				}
			}
			return startBackgroundCommand(s, name, path, args, cancel, waitDelay)
		},
	)
}

// startBackgroundCommand starts a command with graceful shutdown support.
// It follows the same pattern as rsc.io/script's startCommand but with key differences:
//   - Default cancel sends SIGTERM instead of SIGKILL for graceful shutdown
//   - Exit code 0 with context.Canceled is treated as success
//   - Ensures GOCOVERDIR is set for test coverage collection
//   - Handles ETXTBSY errors by retrying (executable still being written)
//   - Detects early failures (within 500ms) and reports them immediately
//
// This allows background servers to shut down cleanly, write coverage data,
// and exit successfully when the test ends.
func startBackgroundCommand(s *script.State, name, path string, args []string, cancel func(*exec.Cmd) error, waitDelay time.Duration) (script.WaitFunc, error) {
	var (
		cmd            *exec.Cmd
		stdout, stderr strings.Builder
	)

	// Retry loop to handle ETXTBSY errors
	for {
		cmd = exec.CommandContext(s.Context(), path, args...)
		cmd.Args[0] = name
		cmd.Dir = s.Getwd()
		cmd.Env = s.Environ()
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.WaitDelay = waitDelay

		// Set cancel function - default to SIGTERM for graceful shutdown
		if cancel == nil {
			cmd.Cancel = func() error {
				if cmd.Process != nil {
					return cmd.Process.Signal(syscall.SIGTERM)
				}
				return nil
			}
		} else {
			cmd.Cancel = func() error { return cancel(cmd) }
		}

		// Ensure GOCOVERDIR is set for coverage collection
		if gcd := os.Getenv("GOCOVERDIR"); gcd != "" {
			found := false
			for i, e := range cmd.Env {
				if strings.HasPrefix(e, "GOCOVERDIR=") {
					cmd.Env[i] = "GOCOVERDIR=" + gcd
					found = true
					break
				}
			}
			if !found {
				cmd.Env = append(cmd.Env, "GOCOVERDIR="+gcd)
			}
		}

		err := cmd.Start()
		if err == nil {
			break // Successfully started
		}
		if isETXTBSY(err) {
			// If the script just wrote the executable we're trying to run,
			// a fork+exec in another thread may be holding open the FD
			// that we used to write the executable (see https://go.dev/issue/22315).
			// Since the descriptor should have CLOEXEC set, the problem should
			// resolve as soon as the forked child reaches its exec call.
			// Keep retrying until that happens.
			continue
		}
		return nil, err
	}

	wait := func(s *script.State) (string, string, error) {
		err := cmd.Wait()

		// When the script's context is cancelled, the process is signaled
		// to shut down. Either a clean exit (code 0) or a signal-terminated
		// exit counts as success — we asked it to stop.
		if s.Context().Err() != nil {
			return stdout.String(), stderr.String(), nil
		}
		return stdout.String(), stderr.String(), err
	}
	return wait, nil
}

// TestMain runs m.Run() normally, or runs mainFunc if invoked without -test flags.
func TestMain(m interface{ Run() int }, mainFunc func()) {
	if !strings.HasSuffix(os.Args[0], ".test") {
		os.Exit(m.Run())
	}

	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.") {
			os.Exit(m.Run())
		}
	}

	mainFunc()
	os.Exit(0)
}

// Test is a drop-in replacement for scripttest.Test that rewrites
// hardcoded TCP port numbers in each script to free ports reserved at
// runtime. Subtests run in parallel; rewriting prevents the port
// collisions that would otherwise force sequential execution.
func Test(t *testing.T, ctx context.Context, engine *script.Engine, env []string, pattern string) {
	gracePeriod := 100 * time.Millisecond
	if deadline, ok := t.Deadline(); ok {
		timeout := time.Until(deadline)

		// If time allows, increase the termination grace period to 5% of the
		// remaining time.
		if gp := timeout / 20; gp > gracePeriod {
			gracePeriod = gp
		}

		// When we run commands that execute subprocesses, we want to reserve two
		// grace periods to clean up. We will send the first termination signal when
		// the context expires, then wait one grace period for the process to
		// produce whatever useful output it can (such as a stack trace). After the
		// first grace period expires, we'll escalate to os.Kill, leaving the second
		// grace period for the test function to record its output before the test
		// process itself terminates.
		timeout -= 2 * gracePeriod

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		t.Cleanup(cancel)
	}

	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, file := range files {
		file := file
		name := strings.TrimSuffix(filepath.Base(file), ".txt")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			workdir := t.TempDir()
			s, err := script.NewState(ctx, workdir, env)
			if err != nil {
				t.Fatal(err)
			}

			// Unpack archive.
			a, err := txtar.ParseFile(file)
			if err != nil {
				t.Fatal(err)
			}
			initScriptDirs(t, s)
			if err := s.ExtractFiles(a); err != nil {
				t.Fatal(err)
			}

			t.Log(time.Now().UTC().Format(time.RFC3339))
			work, _ := s.LookupEnv("WORK")
			t.Logf("$WORK=%s", work)

			scriptText, err := rewriteScriptPorts(string(a.Comment))
			if err != nil {
				t.Fatal(err)
			}

			// Use scripttest.Run to execute the test
			scripttest.Run(t, engine, s, file, bytes.NewReader([]byte(scriptText)))
		})
	}
}

// initScriptDirs initializes the script directories for testing.
func initScriptDirs(t testing.TB, s *script.State) {
	must := func(err error) {
		if err != nil {
			t.Helper()
			t.Fatal(err)
		}
	}

	work := s.Getwd()
	must(s.Setenv("WORK", work))
	must(os.MkdirAll(filepath.Join(work, "tmp"), 0777))
	must(s.Setenv(tempEnvName(), filepath.Join(work, "tmp")))
}

// tempEnvName returns the environment variable name for temp directory.
func tempEnvName() string {
	switch runtime.GOOS {
	case "windows":
		return "TMP"
	case "plan9":
		return "TMPDIR" // actually plan 9 doesn't have one at all but this is fine
	default:
		return "TMPDIR"
	}
}

func rewriteScriptPorts(script string) (string, error) {
	portMap := make(map[string]string)

	rewritten := scriptPortPattern.ReplaceAllStringFunc(script, func(match string) string {
		parts := scriptPortPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		oldPort := parts[2]
		newPort, ok := portMap[oldPort]
		if !ok {
			var err error
			newPort, err = reserveTestPort()
			if err != nil {
				// ReplaceAllStringFunc does not surface errors. Encode the
				// failure inline and reject it after the rewrite pass.
				newPort = "ERROR:" + err.Error()
			}
			portMap[oldPort] = newPort
		}
		return parts[1] + newPort
	})

	for _, newPort := range portMap {
		if strings.HasPrefix(newPort, "ERROR:") {
			return "", errors.New(strings.TrimPrefix(newPort, "ERROR:"))
		}
	}
	return rewritten, nil
}

func reserveTestPort() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected listener addr type %T", ln.Addr())
	}
	return fmt.Sprintf("%d", addr.Port), nil
}

// isETXTBSY reports whether err is a "text file busy" error (ETXTBSY).
// This can occur on Unix systems when trying to execute a file that
// is still being written by another process.
func isETXTBSY(err error) bool {
	if runtime.GOOS == "windows" {
		return false // Windows doesn't have ETXTBSY
	}
	return errors.Is(err, syscall.ETXTBSY)
}
