package iconsets

import "testing"

func TestEmbeddedLibraries(t *testing.T) {
	wantCount := map[string]int{
		"fontawesome": 2860,
		"lucide":      1756,
		"tabler":      5130,
	}
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			lib, err := Load(name)
			if err != nil {
				t.Fatal(err)
			}
			if len(lib.Icons) != wantCount[name] {
				t.Errorf("icons = %d, want %d", len(lib.Icons), wantCount[name])
			}
			if lib.Name != name || lib.Version == "" || lib.Source == "" {
				t.Errorf("incomplete provenance: %#v", lib)
			}
			if lib.License == "" || lib.Attribution == "" {
				t.Error("embedded license or attribution is empty")
			}
		})
	}
}

func TestAliases(t *testing.T) {
	aliases := Aliases("diagram-project")
	for _, want := range []string{"workflow", "sitemap"} {
		found := false
		for _, got := range aliases {
			found = found || got == want
		}
		if !found {
			t.Errorf("Aliases(diagram-project) = %v, missing %q", aliases, want)
		}
	}
}

func TestAliasGroupsAreIsolated(t *testing.T) {
	first := AliasGroups()
	first[0][0] = "changed"

	second := AliasGroups()
	if second[0][0] == "changed" {
		t.Fatal("mutating one alias-group result changed another")
	}
}
