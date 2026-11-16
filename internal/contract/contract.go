package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"contracter/internal/files"
)

// version is the contract file format this code reads and writes.
const version = 1

// Contract is config.contract.json.
type Contract struct {
	Version int               `json:"version"`
	Entries map[string]*Entry `json:"entries"` // by path pattern
}

// Entry is one config entry: what was declared for it and what was seen.
type Entry struct {
	Checks
	// Instances are the values the pattern's placeholders took: ["en", "he"].
	Instances []string          `json:"instances,omitempty"`
	Files     map[string]*Shape `json:"files"` // by logical path
}

// Shape is what the contract holds about one file. Values are only ever
// stored as hashes.
type Shape struct {
	Typed       bool                `json:"typed"`
	Keys        map[string]string   `json:"keys"`                   // key path -> type
	EnvSpecific map[string][]string `json:"env_specific,omitempty"` // key path -> hashes of snapshot values
	Locked      map[string][]string `json:"locked,omitempty"`
}

func Read(file string) (*Contract, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	c, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return c, nil
}

func Parse(data []byte) (*Contract, error) {
	var c Contract
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Version != version {
		return nil, fmt.Errorf("contract version %d, this contracter reads version %d", c.Version, version)
	}
	// A contract is input. Its entry patterns are the only paths that get
	// read, and validEntry keeps those inside the repo.
	for pattern, e := range c.Entries {
		if e == nil {
			return nil, fmt.Errorf("%s: entry is null", pattern)
		}
		if err := validEntry(pattern, e.Checks); err != nil {
			return nil, err
		}
		for logical, s := range e.Files {
			if s == nil {
				return nil, fmt.Errorf("%s: shape is null", logical)
			}
		}
	}
	return &c, nil
}

// Encode renders the contract. The same contract always encodes to the same
// bytes, so it diffs cleanly and staleness is a byte comparison.
func (c *Contract) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	err := enc.Encode(c)
	return buf.Bytes(), err
}

// Loaded holds the files read for each entry, by path pattern.
type Loaded map[string][]files.File

func (c *Contract) LoadFiles(root string, sources []files.Source) (Loaded, error) {
	formats := map[string]string{}
	for pattern, e := range c.Entries {
		formats[pattern] = e.Format
	}
	return loadFiles(root, formats, sources)
}

func loadFiles(root string, formats map[string]string, sources []files.Source) (Loaded, error) {
	// A source that touches no entry is a typo, and would silently leave the
	// file in the repo being read instead.
	for _, s := range sources {
		used := false
		for pattern := range formats {
			used = used || s.Touches(pattern)
		}
		if !used {
			return nil, fmt.Errorf("-source %s matches no entry", s.Path)
		}
	}
	loaded := Loaded{}
	for pattern, formatName := range formats {
		fs, err := files.Load(root, pattern, formatName, sources)
		if err != nil {
			return nil, err
		}
		loaded[pattern] = fs
	}
	return loaded, nil
}
