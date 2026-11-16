// Package contract defines contracter.yaml and the contract built from it,
// builds a contract from files, and checks files against one.
package contract

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"contracter/internal/files"
	"contracter/internal/format"
	"contracter/internal/shape"
)

// Config is contracter.yaml: the files to put under contract.
type Config struct {
	Files []File `yaml:"files"`
}

// File is one entry of contracter.yaml. Everything in Checks is copied into
// the contract as written; the value patterns are turned into hashes there.
type File struct {
	Path        string `yaml:"path"`
	Checks      `yaml:",inline"`
	EnvSpecific shape.Patterns `yaml:"env-specific"` // values that must differ from the snapshot
	Locked      shape.Patterns `yaml:"locked"`       // values that must equal the snapshot
}

// Checks is the part of an entry that a check applies exactly as declared.
type Checks struct {
	Format   string         `yaml:"format" json:"format,omitempty"`
	Optional shape.Patterns `yaml:"optional" json:"optional,omitempty"` // keys that may be absent or extra
	Rules    []Rule         `yaml:"rules" json:"rules,omitempty"`
}

// Rule constrains the values of every key matching Key.
type Rule struct {
	Key   shape.Pattern  `yaml:"key" json:"key"`
	Match *regexp.Regexp `yaml:"match" json:"match,omitempty"`
	Not   *regexp.Regexp `yaml:"not" json:"not,omitempty"`
}

// UnmarshalYAML also accepts a bare path in place of a mapping.
func (f *File) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		f.Path = n.Value
		return nil
	}
	if err := knownFields(n, reflect.TypeFor[File]()); err != nil {
		return err
	}
	type plain File
	return n.Decode((*plain)(f))
}

func (r *Rule) UnmarshalYAML(n *yaml.Node) error {
	if err := knownFields(n, reflect.TypeFor[Rule]()); err != nil {
		return err
	}
	type plain Rule
	if err := n.Decode((*plain)(r)); err != nil {
		return fmt.Errorf("line %d: rule: %w", n.Line, err)
	}
	return nil
}

// knownFields rejects mapping keys that are not fields of t. The decoder's
// own strict mode does not reach into types with an UnmarshalYAML, and a
// misspelled field would silently switch a check off.
func knownFields(n *yaml.Node, t reflect.Type) error {
	known := yamlFields(t)
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i]; !slices.Contains(known, k.Value) {
			return fmt.Errorf("line %d: unknown field %q (want one of: %s)", k.Line, k.Value, strings.Join(known, ", "))
		}
	}
	return nil
}

func yamlFields(t reflect.Type) []string {
	var names []string
	for i := range t.NumField() {
		name, option, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if option == "inline" {
			names = append(names, yamlFields(t.Field(i).Type)...)
		} else {
			names = append(names, name)
		}
	}
	return names
}

func ReadConfig(file string) (*Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// io.EOF is an empty file, which the check below reports.
	if err := dec.Decode(&c); err != nil && err != io.EOF {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if len(c.Files) == 0 {
		return nil, fmt.Errorf("%s: lists no files, so its contract would accept anything", file)
	}
	seen := map[string]bool{}
	for _, f := range c.Files {
		if err := validEntry(f.Path, f.Checks); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if seen[f.Path] {
			return nil, fmt.Errorf("%s: %s is listed twice", file, f.Path)
		}
		seen[f.Path] = true
	}
	return &c, nil
}

// validEntry checks what a config entry and a contract entry have in common.
func validEntry(pattern string, c Checks) error {
	if !files.ValidPath(pattern) {
		return fmt.Errorf("invalid path %q (want a relative path inside the repo, no \"..\" or \"./\")", pattern)
	}
	if _, ok := format.Named(c.Format); c.Format != "" && !ok {
		return fmt.Errorf("%s: unknown format %q", pattern, c.Format)
	}
	for _, r := range c.Rules {
		if r.Key.String() == "" || (r.Match == nil && r.Not == nil) {
			return fmt.Errorf("%s: a rule needs a key and at least one of match, not", pattern)
		}
		// An empty regexp matches everything: as match it checks nothing,
		// as not it fails every value.
		for _, re := range []*regexp.Regexp{r.Match, r.Not} {
			if re != nil && re.String() == "" {
				return fmt.Errorf("%s: rule for %s has an empty regexp", pattern, r.Key)
			}
		}
	}
	return nil
}

func (c *Config) LoadFiles(root string, sources []files.Source) (Loaded, error) {
	formats := map[string]string{}
	for _, f := range c.Files {
		formats[f.Path] = f.Format
	}
	return loadFiles(root, formats, sources)
}

const starterHeader = `# Files contracter keeps under contract. Prune this list, then run:
#   contracter snapshot
#
# An entry is a path, or a mapping:
#   - path: config/app.yaml
#     optional: [debug.*]        # keys that may be absent or extra
#     env-specific: [db.host]    # value must differ from the snapshot
#     locked: [image.tag]        # value must equal the snapshot
#     rules:                     # constraints on values (regexps)
#       - {key: "*.url", match: '^https://'}
#       - {key: image, not: 'docker\.io'}
#     format: yaml               # only if the extension does not say
#
# Paths may use *, ? and **. A {name} placeholder means "everything matching
# here must have the same files and the same keys":
#   public/locales/{lng}/*.json      deploy/values-{env}.yaml
`

// Starter renders a contracter.yaml that lists the given paths and explains
// what else an entry can say.
func Starter(paths []string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(starterHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(map[string][]string{"files": paths})
	return buf.Bytes(), err
}
