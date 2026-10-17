package docversions

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ManifestName is the file name of the manifest, at the root of a
// versioned site.
const ManifestName = "versions.json"

// A Manifest describes a versioned site. Every page loads it when it is
// viewed rather than when it is built, so a frozen old version still
// lists the versions released after it.
type Manifest struct {
	// Default names the version the site root opens.
	Default string `json:"default"`
	// Versions lists the published versions, newest first, with the
	// development version at the top.
	Versions []Version `json:"versions"`
	// Moves maps a version name to the pages that moved between it and
	// the next newer version: each key is a page's path in that
	// version and each value its path in the newer one. Paths are
	// relative to the version root, with no extension and no trailing
	// index, so "guide/" and "guide.html" are both "guide".
	Moves map[string]map[string]string `json:"moves,omitempty"`
	// Aliases maps extra directory names, such as "stable", to the
	// version each holds a copy of.
	Aliases map[string]string `json:"aliases,omitempty"`
	// Banner is the text shown at the top of pages that are not the
	// current release.
	Banner Banner `json:"banner"`
}

// A Version is one published version of a site.
type Version struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	// Path is the version's root, relative to the manifest, ending in
	// a slash.
	Path   string `json:"path"`
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
	// State is "dev" for unreleased docs, "stable" for the newest
	// release, and "old" for every other version. It selects the
	// banner.
	State   string `json:"state"`
	Archive bool   `json:"archive,omitempty"`
	// Hidden versions are published but left out of the switcher.
	Hidden bool `json:"hidden,omitempty"`
}

// Version states.
const (
	StateDev    = "dev"
	StateStable = "stable"
	StateOld    = "old"
)

// Lookup returns the version named name.
func (m *Manifest) Lookup(name string) (Version, bool) {
	for _, v := range m.Versions {
		if v.Name == name {
			return v, true
		}
	}
	return Version{}, false
}

// Marshal returns the manifest as indented JSON.
func (m *Manifest) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding manifest: %w", err)
	}
	return append(data, '\n'), nil
}

// WriteFile writes the manifest to file.
func (m *Manifest) WriteFile(file string) error {
	data, err := m.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}

// PageKey returns the key [Manifest.Moves] uses for the page at the URL
// u, relative to its version root, in a site whose pages end in ext
// (such as ".html", or "" for extensionless URLs). A page's file URL
// and directory URL have the same key: "guide.html", "guide/" and
// "guide/index.html" are all "guide". The version root is "".
func PageKey(u, ext string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	u = strings.Trim(u, "/")
	if ext != "" {
		u = strings.TrimSuffix(u, ext)
	}
	if u == "index" || u == "." {
		return ""
	}
	return strings.TrimSuffix(u, "/index")
}
