package contract

import (
	"fmt"
	"slices"
	"sort"

	"contracter/internal/files"
	"contracter/internal/shape"
)

// Build makes the contract for the files a config lists. The warnings are
// things the config asks for that will have no effect.
func Build(cfg *Config, loaded Loaded) (*Contract, []string, error) {
	con := &Contract{Version: version, Entries: map[string]*Entry{}}
	var warnings []string
	for _, spec := range cfg.Files {
		e, entryWarnings, err := buildEntry(spec, loaded[spec.Path])
		if err != nil {
			return nil, nil, err
		}
		con.Entries[spec.Path] = e
		for _, w := range entryWarnings {
			warnings = append(warnings, spec.Path+": "+w)
		}
	}
	return con, warnings, nil
}

func buildEntry(spec File, loaded []files.File) (*Entry, []string, error) {
	if len(loaded) == 0 {
		return nil, nil, fmt.Errorf("%s: no file found (pass -source if it lives elsewhere)", spec.Path)
	}
	// What comes through a source may be from a secret manager; not even a
	// hash of it belongs in a committed file. One such file switches value
	// recording off for the whole entry, so its instances are treated alike.
	recordable := !slices.ContainsFunc(loaded, func(f files.File) bool { return f.FromSource })

	e := &Entry{Checks: spec.Checks, Files: map[string]*Shape{}}
	for _, f := range loaded {
		if f.Err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f.Path, f.Err)
		}
		if f.Instance != "" && !slices.Contains(e.Instances, f.Instance) {
			e.Instances = append(e.Instances, f.Instance)
		}
		// Instances of a logical file share one shape: the union of their
		// keys. Checking the instances against it shows where they disagree.
		s := e.Files[f.Logical]
		if s == nil {
			s = &Shape{Typed: true, Keys: map[string]string{}, EnvSpecific: map[string][]string{}, Locked: map[string][]string{}}
			e.Files[f.Logical] = s
		}
		s.Typed = s.Typed && f.Typed
		shape.Union(s.Keys, f.Leaves.Types())
		if recordable {
			recordValues(s.EnvSpecific, spec.EnvSpecific, f)
			recordValues(s.Locked, spec.Locked, f)
		}
	}
	sort.Strings(e.Instances)
	return e, ineffective(spec, e, recordable), nil
}
