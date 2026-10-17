package mdvet

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tmc/md2html/internal/docversions"
)

// Versions configures [RunVersions].
type Versions struct {
	// Pattern is the glob tags must match to be published. Empty
	// matches every tag.
	Pattern string
	// Config is the version policy file. Empty means versions.yaml in
	// the directory being vetted.
	Config string
	// Site is how each version is served. Its Root is replaced by the
	// version's own directory.
	Site Site
}

// RunVersions vets the directory dir of a versioned site as md2html
// -versions publishes it: as it is in the working tree and as it was
// at each published tag, then across versions. A tag's diagnostics
// name its files as tag:path, with path relative to the repository
// root, as git does.
//
// The cross-version checks are:
//
//   - version-aliases: an aliases entry in a page's frontmatter must
//     name a page that exists in some version;
//   - version-removed: a page in the newest release must still exist
//     in the working tree, or have been moved, by a rename git detects
//     or an alias, since otherwise links to it in latest break.
func RunVersions(ctx context.Context, dir string, checks []Check, v Versions) ([]Diagnostic, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	repo := docversions.Repo{Dir: dir}
	if !repo.IsRepo(ctx) {
		return nil, fmt.Errorf("versions: %s is not in a git repository", dir)
	}
	top, err := repo.Toplevel(ctx)
	if err != nil {
		return nil, err
	}
	rel, err := docversions.RelPath(top, dir)
	if err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}
	file := v.Config
	if file == "" {
		file = filepath.Join(dir, "versions.yaml")
	}
	cfg, err := docversions.LoadConfig(file)
	if err != nil {
		return nil, err
	}
	tags, err := repo.Tags(ctx, v.Pattern)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	tags = slices.DeleteFunc(tags, func(r docversions.Ref) bool { return !cfg.Publishes(r.Name) })

	site := v.Site
	site.Root = dir
	diags, err := RunSite([]string{dir}, checks, site)
	if err != nil {
		return nil, err
	}

	pages := make(map[string]bool) // source pages in any version
	latest, err := collectFiles([]string{dir})
	if err != nil {
		return nil, err
	}
	have := make(map[string]bool)
	for _, f := range latest {
		r, err := filepath.Rel(dir, f)
		if err != nil {
			return nil, err
		}
		have[filepath.ToSlash(r)] = true
		pages[filepath.ToSlash(r)] = true
	}

	tmp, err := os.MkdirTemp("", "mdvet-versions-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	for _, tag := range tags {
		found, err := repo.Pages(ctx, tag.Commit, rel)
		if err != nil {
			return nil, err
		}
		for _, p := range found {
			pages[p] = true
		}
		if len(found) == 0 {
			continue
		}
		tagDiags, err := vetRef(ctx, repo, top, rel, tag, checks, v.Site, filepath.Join(tmp, tag.Name))
		if err != nil {
			return nil, fmt.Errorf("version %s: %w", tag.Name, err)
		}
		diags = append(diags, tagDiags...)
	}

	aliased := make(map[string]bool)
	for _, f := range latest {
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		fm, ok, err := frontmatter(src)
		if err != nil || !ok {
			continue
		}
		for _, a := range docversions.Aliases(fm) {
			page := docversions.AliasSource(a)
			aliased[page] = true
			if !pages[page] {
				diags = append(diags, Diagnostic{
					File:    f,
					Line:    lineContaining(src, "aliases"),
					Check:   "version-aliases",
					Message: fmt.Sprintf("alias %q names a page in no version", a),
				})
			}
		}
	}

	if len(tags) > 0 {
		newest := tags[0]
		moved, err := repo.Renames(ctx, newest.Commit, "", rel)
		if err != nil {
			return nil, err
		}
		released, err := repo.Pages(ctx, newest.Commit, rel)
		if err != nil {
			return nil, err
		}
		for _, p := range released {
			if have[p] || aliased[p] {
				continue
			}
			if _, ok := moved[p]; ok {
				continue
			}
			diags = append(diags, Diagnostic{
				File:    newest.Name + ":" + path.Join(rel, p),
				Check:   "version-removed",
				Message: fmt.Sprintf("page removed since %s with no move or alias; links to it in latest will break", newest.Name),
			})
		}
	}
	return diags, nil
}

// vetRef runs checks on the directory rel of the repository as of ref,
// checked out into a worktree at wt.
func vetRef(ctx context.Context, repo docversions.Repo, top, rel string, ref docversions.Ref, checks []Check, site Site, wt string) ([]Diagnostic, error) {
	if err := repo.AddWorktree(ctx, wt, ref.Commit); err != nil {
		return nil, err
	}
	defer repo.RemoveWorktree(context.WithoutCancel(ctx), wt)
	dir := filepath.Join(wt, filepath.FromSlash(rel))
	site.Root = dir

	// An include root inside the repository is read at the ref too.
	refChecks := slices.Clone(checks)
	for i, c := range refChecks {
		if ic, ok := c.(IncludeCheck); ok && ic.Root != "" {
			if r, err := docversions.RelPath(top, ic.Root); err == nil {
				refChecks[i] = IncludeCheck{Root: filepath.Join(wt, filepath.FromSlash(r))}
			}
		}
	}
	diags, err := RunSite([]string{dir}, refChecks, site)
	if err != nil {
		return nil, err
	}
	for i := range diags {
		if r, err := filepath.Rel(wt, diags[i].File); err == nil && filepath.IsLocal(r) {
			diags[i].File = ref.Name + ":" + filepath.ToSlash(r)
		}
		diags[i].Message = strings.ReplaceAll(diags[i].Message, wt+string(filepath.Separator), "")
	}
	return diags, nil
}

// lineContaining returns the 1-based line of the first line of src
// that starts with prefix, or 0.
func lineContaining(src []byte, prefix string) int {
	for i, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return i + 1
		}
	}
	return 0
}
