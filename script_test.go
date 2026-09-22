package md2html

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/tmc/md2html/internal/scripttest"
	"rsc.io/script"
)

var borderline = flag.Bool("include-borderline-tests", false, "run borderline tests that may be slow or push limits")

func TestScripts(t *testing.T) {
	exe, _ := os.Executable()
	engine := script.NewEngine()
	engine.Cmds["md2html"] = scripttest.BackgroundCmd(exe, nil, 0)
	engine.Cmds["curl"] = script.Program("curl", nil, 0)
	engine.Cmds["wait-port"] = scripttest.WaitPortCmd()
	// remove Exec:
	delete(engine.Cmds, "exec")

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + t.TempDir(),
	}
	if gcd := os.Getenv("GOCOVERDIR"); gcd != "" {
		env = append(env, "GOCOVERDIR="+gcd)
	}
	scripttest.Test(t, context.Background(), engine, env, "testdata/*.txt")
	if *borderline {
		scripttest.Test(t, context.Background(), engine, env, "testdata/borderline/*.txt")
	}
}
