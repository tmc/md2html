package docversions

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// A Config is a site's version policy: the parts of it that tags cannot
// express. It is read from the development branch only, so the policy
// can change without retagging.
//
// A typical versions.yaml:
//
//	since: v0.1.17
//	exclude: ["v0.1.19"]
//	collapse: minor
//	labels:
//	  v0.2.0: "v0.2.0 (LTS)"
//	archive:
//	  - name: v0.1.16
//	    label: v0.1.16 and earlier
//	    path: archive/v0.1.16
//	banner:
//	  old: "You are reading docs for {version}. The latest release is {latest}."
//	  dev: "These docs describe unreleased changes on main."
type Config struct {
	// Since is the oldest version to publish. Tags below it by
	// semantic version, including its own prereleases, and tags that
	// are not semantic versions are not published. Nor are
	// prereleases, unless Since is one or an Include glob naming
	// prereleases, one containing "-" such as "v1.0.0-rc.*", matches.
	Since string `yaml:"since"`
	// Include lists the tag globs to publish. Empty publishes every tag
	// the tag pattern matches.
	Include []string `yaml:"include"`
	// Exclude lists tag globs not to publish, even if included.
	Exclude []string `yaml:"exclude"`
	// Collapse hides all but the newest patch release of each "minor"
	// or "major" line from the version switcher. They are still built.
	Collapse string `yaml:"collapse"`
	// Labels maps version names to the labels readers see.
	Labels map[string]string `yaml:"labels"`
	// Archive lists versions built elsewhere and copied in verbatim.
	Archive []Archive `yaml:"archive"`
	// Banner holds the banner text shown for each version state.
	Banner Banner `yaml:"banner"`
}

// An Archive is a version md2html did not render: a directory of
// finished HTML, or an http or https URL of a .tar.gz of one, copied to
// the output under Name.
type Archive struct {
	Name  string `yaml:"name"`
	Label string `yaml:"label"`
	Path  string `yaml:"path"`
}

// Banner is the text shown at the top of pages of a version in a given
// state. {version} expands to the version's label and {latest} to a
// link to the default version.
type Banner struct {
	Old string `yaml:"old" json:"old,omitempty"`
	Dev string `yaml:"dev" json:"dev,omitempty"`
}

// DefaultBanner is the banner text used when a configuration names none.
var DefaultBanner = Banner{
	Old: "You are reading the documentation for {version}. Read {latest} for the current documentation.",
	Dev: "You are reading the documentation for unreleased changes.",
}

// LoadConfig reads the configuration at file. A missing file is an
// empty configuration.
func LoadConfig(file string) (*Config, error) {
	cfg := new(Config)
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading version config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", file, err)
	}
	if err := cfg.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return cfg, nil
}

func (c *Config) check() error {
	if c.Since != "" {
		if _, ok := parse(c.Since); !ok {
			return fmt.Errorf("since: %q is not a semantic version", c.Since)
		}
	}
	switch c.Collapse {
	case "", "minor", "major":
	default:
		return fmt.Errorf("collapse: %q is not minor or major", c.Collapse)
	}
	for _, globs := range [][]string{c.Include, c.Exclude} {
		for _, g := range globs {
			if _, err := path.Match(g, ""); err != nil {
				return fmt.Errorf("bad pattern %q: %w", g, err)
			}
		}
	}
	seen := make(map[string]bool)
	for _, a := range c.Archive {
		if a.Name == "" || a.Path == "" {
			return fmt.Errorf("archive: name and path are required")
		}
		if err := CheckName(a.Name); err != nil {
			return fmt.Errorf("archive: %w", err)
		}
		if seen[a.Name] {
			return fmt.Errorf("archive: %s is listed twice", a.Name)
		}
		seen[a.Name] = true
	}
	return nil
}

// CheckName reports whether name can be used as a version's directory.
func CheckName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%q cannot name a version directory", name)
	}
	return nil
}

// Publishes reports whether the tag name is published.
func (c *Config) Publishes(name string) bool {
	if c.Since != "" {
		if Compare(name, c.Since) < 0 {
			return false
		}
		if prerelease(name) && !prerelease(c.Since) && !matchAny(prereleaseGlobs(c.Include), name) {
			return false
		}
	}
	if len(c.Include) > 0 && !matchAny(c.Include, name) {
		return false
	}
	return !matchAny(c.Exclude, name)
}

// prereleaseGlobs returns the globs that name prereleases: those
// containing "-", as a prerelease's name does.
func prereleaseGlobs(globs []string) []string {
	var pre []string
	for _, g := range globs {
		if strings.Contains(g, "-") {
			pre = append(pre, g)
		}
	}
	return pre
}

func matchAny(globs []string, name string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
	}
	return false
}

// Label returns the label readers see for the version name.
func (c *Config) Label(name string) string {
	if l := c.Labels[name]; l != "" {
		return l
	}
	return name
}

// Hidden reports which of names, which are newest first, the switcher
// hides under the configuration's collapse policy: every version but
// the newest of its line.
func (c *Config) Hidden(names []string) map[string]bool {
	if c.Collapse == "" {
		return nil
	}
	hidden := make(map[string]bool)
	seen := make(map[string]bool)
	for _, name := range names {
		v, ok := parse(name)
		if !ok {
			continue
		}
		line := fmt.Sprint(v.nums[0])
		if c.Collapse == "minor" {
			line += "." + fmt.Sprint(v.nums[1])
		}
		if seen[line] {
			hidden[name] = true
		}
		seen[line] = true
	}
	return hidden
}

// BannerText returns the banner text, falling back to [DefaultBanner]
// for each state the configuration leaves empty.
func (c *Config) BannerText() Banner {
	b := c.Banner
	if b.Old == "" {
		b.Old = DefaultBanner.Old
	}
	if b.Dev == "" {
		b.Dev = DefaultBanner.Dev
	}
	return b
}
