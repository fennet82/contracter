// Package format reads config files of every supported format into key paths.
// Adding a format means adding a file here and a row to all.
package format

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"contracter/internal/shape"
)

type Format struct {
	Name string
	// Typed is false when the format can only hold strings, so the types
	// read from it say nothing.
	Typed bool

	// claims reports whether a lower-cased file name is in this format.
	claims func(name string) bool
	// parse returns a tree of map[string]any, []any and scalars.
	parse func([]byte) (any, error)
	// flatLists is set when a list in the tree is only a repeated block or
	// element, which a single occurrence would not be.
	flatLists bool
}

// Detection tries these in order. Dotenv goes by whole file name
// (".env.local"), so it has to be asked before the formats that go by
// extension.
var all = []Format{dotenvFormat, yamlFormat, jsonFormat, tomlFormat, hclFormat, iniFormat, propertiesFormat, xmlFormat}

func (f Format) Read(data []byte) (shape.Leaves, error) {
	tree, err := f.parse(data)
	if err != nil {
		return nil, err
	}
	return shape.Flatten(tree, !f.flatLists), nil
}

func Named(name string) (Format, bool) {
	i := slices.IndexFunc(all, func(f Format) bool { return f.Name == name })
	if i < 0 {
		return Format{}, false
	}
	return all[i], true
}

func Detect(file string) (Format, bool) {
	name := strings.ToLower(path.Base(file))
	i := slices.IndexFunc(all, func(f Format) bool { return f.claims(name) })
	if i < 0 {
		return Format{}, false
	}
	return all[i], true
}

func Names() []string {
	names := make([]string, len(all))
	for i, f := range all {
		names[i] = f.Name
	}
	return names
}

func extension(exts ...string) func(string) bool {
	return func(name string) bool { return slices.Contains(exts, path.Ext(name)) }
}

// canonical rewrites the container types some parsers produce into
// map[string]any and []any.
func canonical(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = canonical(child)
		}
	case map[any]any: // YAML, when a key is not a string
		m := make(map[string]any, len(t))
		for k, child := range t {
			m[fmt.Sprint(k)] = canonical(child)
		}
		return m
	case []any:
		for i, child := range t {
			t[i] = canonical(child)
		}
	case []map[string]any: // TOML arrays of tables
		s := make([]any, len(t))
		for i, child := range t {
			s[i] = canonical(child)
		}
		return s
	}
	return v
}

func stringMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
