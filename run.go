package md2html

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Run renders cfg, writing a single converted document to out or, in
// server and static modes, serving or generating a site. args holds the
// positional arguments; the first names the source, overriding
// [Config.Source].
//
// Run does not change the process working directory: [Config.Chdir]
// names the base that relative paths in cfg resolve against, so a caller
// can run two configurations at once without them interfering. It also
// installs no signal handlers; cancel ctx to stop a running server. The
// md2html command installs its own, so ^C still shuts the server down.
func Run(ctx context.Context, cfg Config, logger *slog.Logger, out io.Writer, args []string) error {
	if logger == nil {
		logger = slog.Default()
	}

	if err := validateFormat(cfg.Format); err != nil {
		return err
	}

	// Handle positional arguments
	if len(args) > 0 {
		cfg.Source = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("too many positional arguments")
	}

	// Configure logger level based on verbose flag
	if cfg.Verbose {
		// Create a new logger with debug level when verbose is enabled
		opts := &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
		handler := slog.NewTextHandler(os.Stderr, opts)
		logger = slog.New(handler)
	}

	site, err := prepareSite(cfg, logger)
	if err != nil {
		return err
	}

	// Run mdvet checks before any rendering. Diagnostics are reported
	// to the logger but never cause Run to fail.
	if cfg.Vet {
		runVet(site, logger)
	}

	// TODO: clean up handling stdin and choosing between modes

	// If -html flag is provided, generate static HTML
	if cfg.HTML != "" {
		if cfg.Base != "" {
			return fmt.Errorf("-base applies to server mode; static output is served at whatever prefix the host uses")
		}
		// Default to .html extension for static builds so files are
		// served with the correct Content-Type by standard HTTP servers.
		if cfg.HTMLExt == "" {
			cfg.HTMLExt = "html"
			site.config.HTMLExt = "html"
		}
		return site.generateStaticHTML(ctx, logger)
	}

	// If -http flag is provided, run server
	if cfg.HTTP != "" {
		logger.Info("Starting server", "address", cfg.HTTP)
		err := runServer(ctx, site, logger)
		// Don't treat context cancellation as an error (graceful shutdown)
		if err == context.Canceled {
			return nil
		}
		return err
	}

	// If source is provided but no mode specified, convert to HTML and output to stdout
	if cfg.Source != "" && cfg.Source != "." {
		// Read the resolved path, but keep naming the document the way
		// the caller did: the name is what local links resolve against.
		content, err := os.ReadFile(site.config.Source)
		if err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}

		// Convert to HTML
		doc, err := parseFrontmatter(string(content))
		if err != nil {
			logger.Error("Error parsing frontmatter", "error", err)
			doc = DocumentData{Content: string(content), Frontmatter: make(map[string]any)}
		}

		html, err := site.markdownToHTML(promoteTitleHeading(doc), cfg.Source)
		if err != nil {
			return err
		}
		fmt.Fprint(out, html)
		return nil
	}

	// Neither -html nor -http provided and no source, show usage
	if fs := NewFlagSet("md2html"); fs != nil {
		fs.Usage()
	}
	return flag.ErrHelp
}

func runServer(ctx context.Context, site *preparedSite, logger *slog.Logger) error {
	if _, err := watchEnabled(site.config.Watch, site.config.Source); err != nil {
		return err
	}
	s := newServer(ctx, site, logger)
	return s.Run(ctx)
}

func watchEnabled(mode, source string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return source != "-", nil
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid watch mode %q (want auto, true, or false)", mode)
	}
}

func formatServerURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	if !strings.Contains(addr, "://") {
		return "http://" + addr
	}
	return addr
}

func openBrowser(ctx context.Context, url string) bool {
	// Skip browser opening if environment variable is set (useful for tests)
	if os.Getenv("MD2HTML_NO_BROWSER") != "" {
		return true
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", url)
	default:
		return false
	}
	return cmd.Start() == nil
}
