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
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	engine := script.NewEngine()
	engine.Cmds["md2html"] = scripttest.BackgroundCmd(exe, nil, 0)
	engine.Cmds["curl"] = script.Program("curl", nil, 0)
	engine.Cmds["git"] = script.Program("git", nil, 0)
	engine.Cmds["wait-port"] = scripttest.WaitPortCmd()
	delete(engine.Cmds, "exec")

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + t.TempDir(),
		// Scripts that commit must not depend on the user's git
		// configuration, which may sign commits or rename branches.
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=md2html",
		"GIT_AUTHOR_EMAIL=md2html@example.com",
		"GIT_COMMITTER_NAME=md2html",
		"GIT_COMMITTER_EMAIL=md2html@example.com",
	}
	if gcd := os.Getenv("GOCOVERDIR"); gcd != "" {
		env = append(env, "GOCOVERDIR="+gcd)
	}
	scripttest.Test(t, context.Background(), engine, env, "testdata/*.txt")
	if *borderline {
		scripttest.Test(t, context.Background(), engine, env, "testdata/borderline/*.txt")
	}
}
