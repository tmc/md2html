package md2html

import (
	"reflect"
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
		{
			name: "templateData",
			typ:  reflect.TypeOf(templateData{}),
			want: []fieldSpec{
				{"Title", "string"},
				{"Content", "template.HTML"},
				{"CustomCSS", "template.CSS"},
				{"ChromaCSS", "template.CSS"},
				{"Verbose", "bool"},
				{"LiveReload", "bool"},
				{"HTMLExt", "string"},
				{"Frontmatter", "map[string]interface {}"},
				{"Version", "string"},
				{"Versions", "[]md2html.GitVersion"},
				{"Search", "bool"},
				{"Nav", "*md2html.NavContext"},
				{"SiteTitle", "string"},
				{"Data", "interface {}"},
				{"IndexFile", "string"},
				{"MermaidTheme", "string"},
				{"MermaidDarkTheme", "string"},
				{"MermaidAutoTheme", "bool"},
				{"FilePath", "string"},
				{"AssetBase", "string"},
				{"RawMDURL", "string"},
				{"HasMath", "bool"},
				{"Description", "string"},
				{"CanonicalURL", "string"},
				{"OpenGraphImage", "string"},
				{"LastUpdated", "string"},
				{"EditURL", "string"},
				{"Assets", "map[string]string"},
				{"Accent", "template.CSS"},
				{"AccentDark", "template.CSS"},
				{"Repo", "string"},
				{"RepoURL", "string"},
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
