package md2html

import (
	"log/slog"
	"strings"

	"github.com/tmc/md2html/internal/mdvet"
)

// runVet executes mdvet over site.config.Source and reports any diagnostics it
// finds via logger. It never returns an error: vet is advisory and
// must not block rendering.
func runVet(site *preparedSite, logger *slog.Logger) {
	if site == nil {
		return
	}
	cfg := site.config
	src := cfg.Source
	if src == "" || src == "-" {
		// stdin or unspecified source: nothing on disk to vet.
		return
	}

	checks, err := selectVetChecks(cfg.VetChecks)
	if err != nil {
		logger.Warn("vet: skipping unknown checks", "error", err)
	}
	if len(checks) == 0 {
		return
	}
	if normalizedFormat(cfg.Format) == "okf" {
		checks = withoutVetCheck(checks, "frontmatter")
	}
	checks = withVetComponents(site, checks)
	checks = withVetIcons(site, checks)

	// md2html already knows the prefix the tree is served under, which
	// is what makes a link written as "/docs/quickstart" resolvable back
	// to the file that renders it.
	vetSite := mdvet.Site{Base: cfg.Base}
	if root, err := sourceRoot(site.base, src); err == nil {
		vetSite.Root = root
	}

	diags, err := mdvet.RunSite([]string{src}, checks, vetSite)
	if err != nil {
		logger.Warn("vet: run failed", "error", err)
		return
	}
	for _, d := range diags {
		logger.Warn("vet",
			"check", d.Check,
			"file", d.File,
			"line", d.Line,
			"message", d.Message,
		)
	}
	if len(diags) > 0 {
		logger.Info("vet: completed with diagnostics", "count", len(diags))
	}
}

func withVetIcons(site *preparedSite, checks []mdvet.Check) []mdvet.Check {
	if site == nil || site.iconDisabled {
		return withoutVetCheck(checks, "icons")
	}
	out := make([]mdvet.Check, len(checks))
	copy(out, checks)
	for i, check := range out {
		if _, ok := check.(mdvet.IconCheck); ok {
			out[i] = mdvet.IconCheck{Resolve: site.hasIcon}
		}
	}
	return out
}

// withVetComponents points the components check at the same registry
// used for rendering, so that -vet and -components agree about which
// component names exist.
func withVetComponents(site *preparedSite, checks []mdvet.Check) []mdvet.Check {
	if site == nil || site.componentRegistry == nil {
		return checks
	}
	out := make([]mdvet.Check, len(checks))
	copy(out, checks)
	for i, c := range out {
		if _, ok := c.(mdvet.ComponentCheck); ok {
			out[i] = mdvet.ComponentCheck{Registry: site.componentRegistry, Configured: true}
		}
	}
	return out
}

func withoutVetCheck(checks []mdvet.Check, name string) []mdvet.Check {
	out := checks[:0]
	for _, check := range checks {
		if check.Name() != name {
			out = append(out, check)
		}
	}
	return out
}

// selectVetChecks returns the mdvet checks named in spec, or every
// check when spec is empty. Unknown names are dropped with the
// returned error describing them; the caller decides how to surface
// that to the user.
func selectVetChecks(spec string) ([]mdvet.Check, error) {
	all := mdvet.AllChecks()
	if spec == "" {
		return all, nil
	}
	byName := make(map[string]mdvet.Check, len(all))
	for _, c := range all {
		byName[c.Name()] = c
	}
	var (
		out     []mdvet.Check
		unknown []string
	)
	for n := range strings.SplitSeq(spec, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if c, ok := byName[n]; ok {
			out = append(out, c)
		} else {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return out, &unknownChecksError{names: unknown}
	}
	return out, nil
}

type unknownChecksError struct{ names []string }

func (e *unknownChecksError) Error() string {
	return "unknown checks: " + strings.Join(e.names, ", ")
}
