package cli

import (
	"flag"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"contracter/internal/files"
	"contracter/internal/format"
)

const (
	defaultConfig   = "contracter.yaml"
	defaultContract = "config.contract.json"
)

// invocation is one run of a command: what its flags said and where it may
// print.
type invocation struct {
	root       string
	configFile string
	contract   string
	sources    []files.Source
	checkOnly  bool
	output     printer

	info io.Writer // progress and results for people
	warn io.Writer
}

func (inv *invocation) inRoot(file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(inv.root, file)
}

// flagSet declares the flags every command takes, then the command's own.
// Each flag stores straight into inv, validating as it goes.
func flagSet(cmd command, inv *invocation, stderr io.Writer) *flag.FlagSet {
	fl := flag.NewFlagSet(cmd.name, flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.Usage = func() { usage(stderr, fl) }

	fl.StringVar(&inv.root, "C", ".", "repository root `dir`")
	fl.StringVar(&inv.configFile, "config", defaultConfig, "the `file` listing what to put under contract")
	fl.StringVar(&inv.contract, "contract", defaultContract, "the contract `file`")

	outputs := strings.Join(slices.Sorted(maps.Keys(printers)), "|")
	fl.Func("o", "print deviations as `"+outputs+"` (default \"text\")", func(name string) error {
		var ok bool
		if inv.output, ok = printers[name]; !ok {
			return fmt.Errorf("want one of %s", outputs)
		}
		return nil
	})
	fl.Func("source", "read contract path (or directory) P from X instead of the repo:\n`P=[FORMAT:]X`. X is a file, a directory or a pipe. Repeatable.\nFORMAT is one of: "+strings.Join(format.Names(), " "),
		func(arg string) error {
			s, err := files.ParseSource(arg)
			inv.sources = append(inv.sources, s)
			return err
		})

	if cmd.flags != nil {
		cmd.flags(fl, inv)
	}
	return fl
}

func snapshotFlags(fl *flag.FlagSet, inv *invocation) {
	fl.BoolVar(&inv.checkOnly, "check", false, "write nothing, fail if the contract is out of date")
}

// parse turns a command's arguments into an invocation. On error it has
// already told the user what is wrong.
func parse(cmd command, args []string, stdout, stderr io.Writer) (*invocation, error) {
	inv := &invocation{output: printers["text"], info: stdout, warn: stderr}
	fl := flagSet(cmd, inv, stderr)
	if err := fl.Parse(args); err != nil {
		return nil, err
	}
	if fl.NArg() > 0 {
		err := fmt.Errorf("unexpected argument %q; run contracter -h", fl.Arg(0))
		fmt.Fprintln(stderr, "contracter:", err)
		return nil, err
	}
	if inv.output.document {
		inv.info = stderr
	}
	return inv, nil
}
