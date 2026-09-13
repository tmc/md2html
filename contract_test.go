package md2html

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPublicContractFields(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want []fieldSpec
	}{
		{
			name: "RenderOptions",
			typ:  reflect.TypeOf(RenderOptions{}),
			want: []fieldSpec{
				{"Nav", "*md2html.NavContext"},
				{"SiteTitle", "string"},
				{"Data", "interface {}"},
				{"FilePath", "string"},
				{"Version", "string"},
				{"Versions", "[]md2html.GitVersion"},
				{"RawMDURL", "string"},
				{"Description", "string"},
				{"EditURL", "string"},
				{"LastUpdated", "string"},
				{"Assets", "map[string]string"},
				{"Accent", "string"},
				{"AccentDark", "string"},
				{"Repo", "string"},
				{"RepoURL", "string"},
				{"NavLinks", "[]md2html.SiteLink"},
				{"Stars", "string"},
				{"ShowStars", "bool"},
			},
		},
		{
			name: "DocumentData",
			typ:  reflect.TypeOf(DocumentData{}),
			want: []fieldSpec{
				{"Content", "string"},
				{"Frontmatter", "map[string]interface {}"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldsOf(tt.typ)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("contract fields changed\n got: %#v\nwant: %#v", got, tt.want)
			}
		})
	}
}

func TestTemplateContractFields(t *testing.T) {
	want := map[string]string{
		"Title":             "string",
		"Content":           "template.HTML",
		"CustomCSS":         "template.CSS",
		"ChromaCSS":         "template.CSS",
		"Verbose":           "bool",
		"LiveReload":        "bool",
		"HTMLExt":           "string",
		"Frontmatter":       "map[string]interface {}",
		"Version":           "string",
		"Versions":          "[]md2html.GitVersion",
		"Search":            "bool",
		"Nav":               "*md2html.NavContext",
		"SiteTitle":         "string",
		"Data":              "interface {}",
		"IndexFile":         "string",
		"MermaidTheme":      "string",
		"MermaidDarkTheme":  "string",
		"MermaidAutoTheme":  "bool",
		"FilePath":          "string",
		"AssetBase":         "string",
		"RawMDURL":          "string",
		"HasMath":           "bool",
		"Description":       "string",
		"CanonicalURL":      "string",
		"OpenGraphImage":    "string",
		"OpenGraphImageAlt": "string",
		"OpenGraphType":     "string",
		"LastUpdated":       "string",
		"EditURL":           "string",
		"Assets":            "map[string]string",
		"Accent":            "template.CSS",
		"AccentDark":        "template.CSS",
		"Repo":              "string",
		"RepoURL":           "string",
		"Stars":             "string",
		"ShowStars":         "bool",
		"IconAttribution":   "template.HTML",
		"NavLinks":          "[]md2html.SiteLink",
	}
	typ := reflect.TypeOf(templateData{})
	got := make(map[string]string, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		got[f.Name] = f.Type.String()
	}
	for name, wantType := range want {
		gotType, ok := got[name]
		if !ok {
			t.Errorf("templateData missing field %s", name)
		} else if gotType != wantType {
			t.Errorf("templateData field %s has type %s, want %s", name, gotType, wantType)
		}
	}
}

func TestCustomTemplateExecution(t *testing.T) {
	dir := t.TempDir()
	const customLayout = `{{define "layout"}}
Title={{.Title}}
Content={{.Content}}
SiteTitle={{.SiteTitle}}
FilePath={{.FilePath}}
Description={{.Description}}
EditURL={{.EditURL}}
LastUpdated={{.LastUpdated}}
Repo={{.Repo}}
RepoURL={{.RepoURL}}
Stars={{.Stars}}
ShowStars={{.ShowStars}}
Search={{.Search}}
Verbose={{.Verbose}}
LiveReload={{.LiveReload}}
HTMLExt={{.HTMLExt}}
IndexFile={{.IndexFile}}
MermaidTheme={{.MermaidTheme}}
MermaidDarkTheme={{.MermaidDarkTheme}}
MermaidAutoTheme={{.MermaidAutoTheme}}
AssetBase={{.AssetBase}}
RawMDURL={{.RawMDURL}}
HasMath={{.HasMath}}
CanonicalURL={{.CanonicalURL}}
OpenGraphImage={{.OpenGraphImage}}
OpenGraphImageAlt={{.OpenGraphImageAlt}}
OpenGraphType={{.OpenGraphType}}
Accent={{.Accent}}
AccentDark={{.AccentDark}}
IconAttribution={{.IconAttribution}}
{{end}}`
	if err := os.WriteFile(filepath.Join(dir, "layout.html"), []byte(customLayout), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		TemplateDir: dir,
		Title:       "Test Site",
	}
	opts := RenderOptions{
		SiteTitle:   "Test Site",
		FilePath:    "test.md",
		Description: "A test document",
		Repo:        "owner/repo",
		RepoURL:     "https://github.com/owner/repo",
		Stars:       "100",
		ShowStars:   true,
	}
	html, err := renderTemplateWithOptions(cfg, "<p>hello</p>", "Page Title", "", false, nil, opts)
	if err != nil {
		t.Fatalf("renderTemplateWithOptions failed: %v", err)
	}
	if !strings.Contains(html, "Title=Page Title") {
		t.Errorf("expected Title=Page Title, got %s", html)
	}
	if !strings.Contains(html, "Content=<p>hello</p>") {
		t.Errorf("expected Content=<p>hello</p>, got %s", html)
	}
	if !strings.Contains(html, "Repo=owner/repo") {
		t.Errorf("expected Repo=owner/repo, got %s", html)
	}
}

func TestConfigHasNoUnexportedFields(t *testing.T) {
	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			t.Errorf("Config has unexported runtime field %q; Config must hold inputs only", f.Name)
		}
	}
}

func TestPreparedSiteIsolation(t *testing.T) {
	site1, err := prepareSite(Config{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	site2, err := prepareSite(Config{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if site1.iconMissing == site2.iconMissing {
		t.Fatal("site1 and site2 share the same iconMissing map")
	}
	site1.warnMissingIcon("missing-icon", "")
	if _, loaded := site2.iconMissing.Load("/missing-icon"); loaded {
		t.Fatal("site2 observed missing icon recorded in site1")
	}
}

type fieldSpec struct {
	Name string
	Type string
}

func fieldsOf(typ reflect.Type) []fieldSpec {
	out := make([]fieldSpec, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		out[i] = fieldSpec{f.Name, f.Type.String()}
	}
	return out
}
