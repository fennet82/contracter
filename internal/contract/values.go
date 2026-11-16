package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"

	"contracter/internal/files"
	"contracter/internal/shape"
)

// The value checks: env-specific and locked compare a key's values with the
// ones hashed at snapshot, rules compare them with regexps. They describe a
// copy being checked, so the files a snapshot is taken from need not pass.

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// recordValues hashes the values of the keys a value check selects.
func recordValues(hashes map[string][]string, selected shape.Patterns, f files.File) {
	for k, leaf := range f.Leaves {
		if len(leaf.Values) == 0 || !selected.Match(k) {
			continue
		}
		for _, v := range leaf.Values {
			if h := hash(v); !slices.Contains(hashes[k], h) {
				hashes[k] = append(hashes[k], h)
			}
		}
		sort.Strings(hashes[k])
	}
}

func (p pair) valueDeviations() []Deviation {
	var devs []Deviation
	inSnapshot := func(hashes []string) func(string) bool {
		return func(v string) bool { return slices.Contains(hashes, hash(v)) }
	}
	for k, hashes := range p.recorded.EnvSpecific {
		if leaf := p.found.Leaves[k]; leaf != nil && slices.ContainsFunc(leaf.Values, inSnapshot(hashes)) {
			devs = append(devs, valueUnchanged(p.name, k))
		}
	}
	for k, hashes := range p.recorded.Locked {
		if leaf := p.found.Leaves[k]; leaf != nil && !all(leaf.Values, inSnapshot(hashes)) {
			devs = append(devs, valueChanged(p.name, k))
		}
	}
	for _, r := range p.entry.Rules {
		for k, leaf := range p.found.Leaves {
			if !r.Key.Match(k) {
				continue
			}
			if r.Match != nil && !all(leaf.Values, r.Match.MatchString) {
				devs = append(devs, ruleMismatch(p.name, k, r.Match.String()))
			}
			if r.Not != nil && slices.ContainsFunc(leaf.Values, r.Not.MatchString) {
				devs = append(devs, ruleForbidden(p.name, k, r.Not.String()))
			}
		}
	}
	return devs
}

func all(values []string, ok func(string) bool) bool {
	return !slices.ContainsFunc(values, func(v string) bool { return !ok(v) })
}

// ineffective lists the value checks a config entry declares that cannot do
// anything. Each is almost always a mistake that would go unnoticed, because
// a check that does nothing never fails.
func ineffective(spec File, e *Entry, recordable bool) []string {
	var warnings []string
	if !recordable && len(spec.EnvSpecific)+len(spec.Locked) > 0 {
		warnings = append(warnings, "read through -source, so its values are not recorded; env-specific and locked have no effect")
	}
	matchesNothing := func(p shape.Pattern) bool {
		for _, s := range e.Files {
			for k := range s.Keys {
				if p.Match(k) {
					return false
				}
			}
		}
		return true
	}
	var ruleKeys shape.Patterns
	for _, r := range spec.Rules {
		ruleKeys = append(ruleKeys, r.Key)
	}
	for _, declared := range []struct {
		field    string
		patterns shape.Patterns
	}{{"env-specific", spec.EnvSpecific}, {"locked", spec.Locked}, {"rule", ruleKeys}} {
		for _, p := range declared.patterns {
			if matchesNothing(p) {
				warnings = append(warnings, declared.field+" "+p.String()+" matches no key")
			}
		}
	}
	return warnings
}
