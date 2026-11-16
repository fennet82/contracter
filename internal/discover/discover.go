// Package discover finds the config files of a repo, to start a
// contracter.yaml from.
package discover

import (
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"

	"contracter/internal/contract"
	"contracter/internal/format"
)

// Entries returns the contracter.yaml entries that cover the config files
// under root, leaving out the named files (contracter's own).
func Entries(root string, except ...string) ([]string, error) {
	tree := os.DirFS(root)
	ignore, err := loadIgnore(tree)
	if err != nil {
		return nil, err
	}
	var found []string
	err = fs.WalkDir(tree, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// The root is where the search starts, whatever it is called.
		if p == "." {
			return nil
		}
		if d.IsDir() {
			if ignore.ignored(p, true) || isHelmTemplates(tree, p) {
				return fs.SkipDir
			}
			return nil
		}
		_, isConfig := format.Detect(d.Name())
		if d.Type().IsRegular() && isConfig && !ignore.ignored(p, false) && !slices.Contains(except, d.Name()) {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return collapseLayouts(root, found), nil
}

// Helm templates are not YAML until they are rendered.
func isHelmTemplates(tree fs.FS, dir string) bool {
	if path.Base(dir) != "templates" {
		return false
	}
	_, err := fs.Stat(tree, path.Join(path.Dir(dir), "Chart.yaml"))
	return err == nil
}

// collapseLayouts replaces sibling directories that already hold the same
// files with the same keys (locales, per-environment folders) by a single
// "parent/{name}/*.ext" entry, which keeps them that way from then on.
func collapseLayouts(root string, found []string) []string {
	candidates := map[string][]string{} // pattern -> the files it would replace
	for _, p := range found {
		if dir, ext := path.Dir(p), path.Ext(p); dir != "." && ext != "" {
			pattern := path.Join(path.Dir(dir), "{name}", "*"+ext)
			candidates[pattern] = append(candidates[pattern], p)
		}
	}
	for pattern, members := range candidates {
		if inParity(root, pattern, len(members)) {
			found = slices.DeleteFunc(found, func(p string) bool { return slices.Contains(members, p) })
			found = append(found, pattern)
		}
	}
	sort.Strings(found)
	return found
}

// inParity reports whether a pattern covers exactly the wanted number of
// files, in at least two instances, that already agree with each other.
func inParity(root, pattern string, want int) bool {
	cfg := &contract.Config{Files: []contract.File{{Path: pattern}}}
	loaded, err := cfg.LoadFiles(root, nil)
	// More matches than wanted means the pattern would also pull in files
	// that were skipped on purpose.
	if err != nil || len(loaded[pattern]) != want {
		return false
	}
	con, _, err := contract.Build(cfg, loaded)
	return err == nil && len(con.Entries[pattern].Instances) >= 2 && len(con.CheckShape(loaded)) == 0
}
