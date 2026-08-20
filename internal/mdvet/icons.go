package mdvet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/tmc/md2html/internal/iconsets"
	"github.com/tmc/md2html/internal/markdown/components"
	"github.com/yuin/goldmark/ast"
)

// IconCheck reports icon names and styles that the selected icon set
// cannot render. Resolve, when set, supplies the renderer's exact lookup.
type IconCheck struct {
	Resolve func(name, style string) bool
}

func (IconCheck) Name() string { return "icons" }

func (c IconCheck) Check(doc *Document) ([]Diagnostic, error) {
	config, library := documentIcons(doc.File)
	var refs []iconRef
	fm, ok, err := frontmatter(doc.Source)
	if ok && err == nil {
		if name, _ := fm["icon"].(string); name != "" {
			style, _ := fm["iconType"].(string)
			refs = append(refs, iconRef{name, style, 1})
		}
	}
	err = ast.Walk(doc.Tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *components.Node:
			if name := n.Attrs["icon"]; name != "" {
				refs = append(refs, iconRef{name, n.Attrs["iconType"], n.OpenLine})
			}
		case *components.Inline:
			if !n.Closing {
				if name := n.Attrs["icon"]; name != "" {
					refs = append(refs, iconRef{name, n.Attrs["iconType"], 1})
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	diags := c.checkConfig(doc.env, config, library)
	for _, ref := range refs {
		if c.Resolve != nil {
			if !c.Resolve(ref.name, ref.style) {
				diags = append(diags, iconDiagnostic(doc.File, ref.line, fmt.Sprintf("icon %q is not in the configured icon set", ref.name)))
			}
			continue
		}
		if library == "fontawesome" && ref.style != "" && !freeFontAwesomeStyle(ref.style) {
			diags = append(diags, iconDiagnostic(doc.File, ref.line, fmt.Sprintf("Font Awesome style %q is not available in Font Awesome Free", ref.style)))
			continue
		}
		if !knownIcon(library, ref.name, ref.style) {
			diags = append(diags, iconDiagnostic(doc.File, ref.line, fmt.Sprintf("icon %q is not in the %s library", ref.name, library)))
		}
	}
	return diags, nil
}

// configIconRE matches an icon name in docs.json. The file is scanned a
// line at a time rather than decoded, because an icon may sit on a
// group, an anchor, a tab, or a navbar link, and what makes the
// diagnostic worth reading is the line it is on.
var configIconRE = regexp.MustCompile(`"icon"\s*:\s*"([^"]+)"`)

// checkConfig reports the icons named in the docs.json at config that
// the library cannot draw. The file governs every page under it, so it
// is checked once per run rather than once per document.
func (c IconCheck) checkConfig(e *env, config, library string) []Diagnostic {
	if config == "" || e == nil || e.configs[config] {
		return nil
	}
	e.configs[config] = true
	data, err := os.ReadFile(config)
	if err != nil {
		return nil
	}
	var diags []Diagnostic
	for i, line := range strings.Split(string(data), "\n") {
		m := configIconRE.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		name := line[m[2]:m[3]]
		if c.Resolve != nil {
			if !c.Resolve(name, "") {
				diags = append(diags, iconDiagnostic(config, i+1, fmt.Sprintf("icon %q is not in the configured icon set", name)))
			}
			continue
		}
		if !knownIcon(library, name, "") {
			diags = append(diags, iconDiagnostic(config, i+1, fmt.Sprintf("icon %q is not in the %s library", name, library)))
		}
	}
	return diags
}

type iconRef struct {
	name, style string
	line        int
}

func iconDiagnostic(file string, line int, message string) Diagnostic {
	return Diagnostic{File: file, Line: line, Col: 1, Check: "icons", Message: message}
}

func freeFontAwesomeStyle(style string) bool {
	return style == "solid" || style == "regular" || style == "brands"
}

var (
	iconLibrariesOnce sync.Once
	iconLibraries     map[string]iconsets.Library
)

func knownIcon(library, name, style string) bool {
	iconLibrariesOnce.Do(func() {
		iconLibraries = make(map[string]iconsets.Library)
		for _, name := range iconsets.Names() {
			lib, err := iconsets.Load(name)
			if err == nil {
				iconLibraries[name] = lib
			}
		}
	})
	lib, ok := iconLibraries[library]
	if !ok {
		return false
	}
	names := append([]string{name}, iconsets.Aliases(name)...)
	for _, name := range names {
		if library != "fontawesome" {
			if _, ok := lib.Icons[name]; ok {
				return true
			}
			continue
		}
		if style != "" {
			if _, ok := lib.Icons[style+"/"+name]; ok {
				return true
			}
			continue
		}
		for _, style := range []string{"solid", "regular", "brands"} {
			if _, ok := lib.Icons[style+"/"+name]; ok {
				return true
			}
		}
	}
	return false
}

// documentIcons reports the docs.json that governs file, and the icon
// library it selects. Mintlify defaults to Font Awesome, which is also
// what a tree with no docs.json gets, and what an unreadable or
// unrecognized setting falls back to.
func documentIcons(file string) (config, library string) {
	dir, err := filepath.Abs(filepath.Dir(file))
	if err != nil {
		return "", "fontawesome"
	}
	for {
		config := filepath.Join(dir, "docs.json")
		data, err := os.ReadFile(config)
		if err == nil {
			var cfg struct {
				Icons struct {
					Library string `json:"library"`
				} `json:"icons"`
			}
			if json.Unmarshal(data, &cfg) == nil {
				switch strings.ToLower(cfg.Icons.Library) {
				case "lucide", "tabler", "fontawesome":
					return config, strings.ToLower(cfg.Icons.Library)
				}
			}
			return config, "fontawesome"
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "fontawesome"
		}
		dir = parent
	}
}
