package contract

import (
	"slices"

	"contracter/internal/files"
	"contracter/internal/shape"
)

// CheckShape reports where files deviate from the contract's layout and key
// shapes. Any copy of the files must pass it, including the one the contract
// was built from.
func (c *Contract) CheckShape(loaded Loaded) []Deviation {
	return c.check(loaded, pair.shapeDeviations)
}

// Check is CheckShape plus the value checks.
func (c *Contract) Check(loaded Loaded) []Deviation {
	return c.check(loaded, pair.shapeDeviations, pair.valueDeviations)
}

func (c *Contract) check(loaded Loaded, checks ...func(pair) []Deviation) []Deviation {
	pairs, devs := c.pair(loaded)
	for _, p := range pairs {
		for _, check := range checks {
			devs = append(devs, check(p)...)
		}
	}
	return sorted(devs)
}

// pair is a file the contract describes together with the file that was read.
type pair struct {
	name     string
	entry    *Entry
	recorded *Shape
	found    files.File
}

// pair matches every file the contract expects with what was loaded, and
// reports the files that are missing, unreadable, or not expected.
func (c *Contract) pair(loaded Loaded) ([]pair, []Deviation) {
	var pairs []pair
	var devs []Deviation
	for pattern, e := range c.Entries {
		found := map[string]map[string]files.File{} // instance -> logical path -> file
		instances := slices.Clone(e.Instances)
		if len(instances) == 0 {
			// Without placeholders there is exactly one, unnamed, instance.
			instances = []string{""}
		}
		for _, f := range loaded[pattern] {
			if found[f.Instance] == nil {
				found[f.Instance] = map[string]files.File{}
			}
			found[f.Instance][f.Logical] = f
			// An instance the contract has never seen must still conform.
			if !slices.Contains(instances, f.Instance) {
				instances = append(instances, f.Instance)
			}
		}
		for _, instance := range instances {
			for logical, s := range e.Files {
				name := files.Concrete(logical, instance)
				switch f, ok := found[instance][logical]; {
				case !ok:
					devs = append(devs, fileMissing(name))
				case f.Err != nil:
					devs = append(devs, unreadable(name, f.Err))
				default:
					pairs = append(pairs, pair{name, e, s, f})
				}
			}
			for logical, f := range found[instance] {
				if e.Files[logical] == nil {
					devs = append(devs, unexpectedFile(f.Path))
				}
			}
		}
	}
	return pairs, devs
}

func (p pair) shapeDeviations() []Deviation {
	var devs []Deviation
	// A format that only holds strings cannot be type-checked.
	typed := p.recorded.Typed && p.found.Typed
	diff := shape.Compare(p.recorded.Keys, p.found.Leaves.Types(), typed, p.entry.Optional)
	for k, typ := range diff.Missing {
		devs = append(devs, missingKey(p.name, k, typ))
	}
	for _, k := range diff.Unknown {
		devs = append(devs, unknownKey(p.name, k))
	}
	for k, types := range diff.Changed {
		devs = append(devs, typeChanged(p.name, k, types[0], types[1]))
	}
	return devs
}
