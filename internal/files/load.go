package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"

	"contracter/internal/format"
	"contracter/internal/shape"
)

// File is one file an entry covers, read or not.
type File struct {
	Path     string // contract path: public/locales/he/auth.json
	Logical  string // placeholders kept: public/locales/{lng}/auth.json
	Instance string // placeholder values: he
	// FromSource is set when the file was read through a Source, so its
	// content may come from somewhere less public than the repo.
	FromSource bool

	Typed  bool
	Leaves shape.Leaves
	// Err is why the file could not be read; Typed and Leaves are then unset.
	Err error
}

// Load reads every file that matches an entry's pattern. formatName, when
// set, overrides detection by file name.
func Load(root, pattern, formatName string, sources []Source) ([]File, error) {
	p := compile(pattern)

	// Files can be in the pattern's own directory, and wherever a source
	// re-points something inside that directory.
	dirs := []string{p.dir}
	for _, s := range sources {
		if s.Path != p.dir && under(s.Path, p.dir) {
			dirs = append(dirs, s.Path)
		}
	}
	var found []string
	for _, dir := range dirs {
		location, src := locate(root, dir, sources)
		// Whoever runs the command chose a source, so links in it are
		// followed: mounted config is often made of them. The repo is input,
		// and a link there must not lead contracter elsewhere.
		err := walk(location, src != nil, func(rel string) {
			file := path.Join(dir, rel)
			// A file that a more specific source re-points is not this one.
			if owner, _ := locate(root, file, sources); filepath.Clean(owner) == filepath.Clean(filepath.Join(location, rel)) {
				found = append(found, file)
			}
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(found)
	found = slices.Compact(found) // the same source given twice

	var files []File
	for _, name := range found {
		logical, instance, ok := p.match(name)
		if !ok {
			continue
		}
		location, src := locate(root, name, sources)
		f := File{Path: name, Logical: logical, Instance: instance, FromSource: src != nil}
		as := formatName
		if src != nil && src.Format != "" {
			as = src.Format
		}
		f.Typed, f.Leaves, f.Err = read(location, name, as)
		files = append(files, f)
	}
	return files, nil
}

func read(location, name, formatName string) (bool, shape.Leaves, error) {
	fm, ok := format.Named(formatName)
	if formatName == "" {
		fm, ok = format.Detect(name)
	}
	if !ok {
		return false, nil, errors.New("unknown format; set format: for this file")
	}
	data, err := os.ReadFile(location)
	if err != nil {
		return false, nil, fmt.Errorf("cannot read: %w", err)
	}
	leaves, err := fm.Read(data)
	if err != nil {
		return false, nil, fmt.Errorf("cannot parse as %s: %w", fm.Name, err)
	}
	return fm.Typed, leaves, nil
}

// walk calls fn with the slash-separated path of every regular file under
// location, relative to it; with "" when location is itself a file or pipe.
// A location that does not exist has no files. With followLinks, a link to a
// regular file counts as one.
func walk(location string, followLinks bool, fn func(rel string)) error {
	info, err := os.Stat(location)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		fn("")
		return nil
	}
	return filepath.WalkDir(location, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		regular := d.Type().IsRegular()
		if followLinks && d.Type()&fs.ModeSymlink != 0 {
			target, err := os.Stat(p)
			regular = err == nil && target.Mode().IsRegular()
		}
		if regular {
			rel, _ := filepath.Rel(location, p)
			fn(filepath.ToSlash(rel))
		}
		return nil
	})
}
