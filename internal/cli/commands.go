package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"contracter/internal/contract"
	"contracter/internal/discover"
)

func runInit(inv *invocation) ([]contract.Deviation, error) {
	configFile := inv.inRoot(inv.configFile)
	if _, err := os.Stat(configFile); err == nil {
		return nil, fmt.Errorf("%s already exists", configFile)
	}
	// Contracter's own files are YAML and JSON too, under their usual names
	// or the ones given.
	entries, err := discover.Entries(inv.root, defaultConfig, defaultContract, filepath.Base(inv.configFile), filepath.Base(inv.contract))
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no config files found under %s (see %s for what is left out)", inv.root, discover.IgnoreFile)
	}
	starter, err := contract.Starter(entries)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(configFile, starter, 0o644); err != nil {
		return nil, err
	}
	fmt.Fprintf(inv.info, "wrote %s with %d entries; prune it, then run: contracter snapshot\n", configFile, len(entries))
	return nil, nil
}

func runSnapshot(inv *invocation) ([]contract.Deviation, error) {
	cfg, err := contract.ReadConfig(inv.inRoot(inv.configFile))
	if err != nil {
		return nil, err
	}
	loaded, err := cfg.LoadFiles(inv.root, inv.sources)
	if err != nil {
		return nil, err
	}
	con, warnings, err := contract.Build(cfg, loaded)
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		fmt.Fprintln(inv.warn, "warning:", w)
	}
	encoded, err := con.Encode()
	if err != nil {
		return nil, err
	}

	// The files a contract is built from must satisfy it too. They do not
	// when the instances of a {placeholder} entry disagree with each other.
	disagreements := con.CheckShape(loaded)

	contractFile := inv.inRoot(inv.contract)
	if inv.checkOnly {
		stale, err := staleness(inv, encoded, loaded)
		if err != nil || stale != nil {
			return stale, err
		}
		if len(disagreements) == 0 {
			fmt.Fprintf(inv.info, "ok: %s is up to date\n", contractFile)
		}
		return disagreements, nil
	}
	if err := os.WriteFile(contractFile, encoded, 0o644); err != nil {
		return nil, err
	}
	shapes, keys := 0, 0
	for _, e := range con.Entries {
		for _, s := range e.Files {
			shapes++
			keys += len(s.Keys)
		}
	}
	fmt.Fprintf(inv.info, "wrote %s: %d file shape(s), %d key(s)\n", contractFile, shapes, keys)
	return disagreements, nil
}

// staleness compares the contract on disk with a freshly built one. If they
// differ it returns what moved, as far as the old contract can tell, followed
// by a deviation saying the contract is stale.
func staleness(inv *invocation, fresh []byte, loaded contract.Loaded) ([]contract.Deviation, error) {
	committed, err := os.ReadFile(inv.inRoot(inv.contract))
	if errors.Is(err, os.ErrNotExist) {
		return []contract.Deviation{contract.NotWritten(inv.contract)}, nil
	}
	if err != nil {
		return nil, err
	}
	if bytes.Equal(committed, fresh) {
		return nil, nil
	}
	var devs []contract.Deviation
	if old, err := contract.Parse(committed); err == nil {
		devs = old.CheckShape(loaded)
	}
	return append(devs, contract.Stale(inv.contract)), nil
}

func runCheck(inv *invocation) ([]contract.Deviation, error) {
	con, err := contract.Read(inv.inRoot(inv.contract))
	if err != nil {
		return nil, err
	}
	loaded, err := con.LoadFiles(inv.root, inv.sources)
	if err != nil {
		return nil, err
	}
	devs := con.Check(loaded)
	if len(devs) == 0 {
		n := 0
		for _, fs := range loaded {
			n += len(fs)
		}
		fmt.Fprintf(inv.info, "ok: %d file(s) match the contract\n", n)
	}
	return devs, nil
}
