// Package cli is the contracter command line: it parses arguments, runs a
// command, prints the result and picks the exit status.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"

	"contracter/internal/contract"
)

const (
	exitOK         = 0
	exitDeviations = 1
	exitError      = 2
)

type command struct {
	name    string
	summary string
	flags   func(*flag.FlagSet, *invocation) // the flags only this command has
	run     func(*invocation) ([]contract.Deviation, error)
}

var commands = []command{
	{"init", "write a starter contracter.yaml listing the config files found", nil, runInit},
	{"snapshot", "read the files in contracter.yaml and write the contract;\n             with -check, write nothing and fail if the contract is out of date", snapshotFlags, runSnapshot},
	{"check", "compare the config files against the contract", nil, runCheck},
}

// Run executes a contracter command line and returns the exit status.
func Run(args []string, stdout, stderr io.Writer) int {
	cmd, ok := findCommand(args)
	if !ok {
		usage(stderr, flagSet(command{}, &invocation{}, stderr))
		if wantsHelp(args) {
			return exitOK
		}
		return exitError
	}
	inv, err := parse(cmd, args[1:], stdout, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		return exitError // parse has said why
	}
	devs, err := cmd.run(inv)
	if err != nil {
		fmt.Fprintln(stderr, "contracter:", err)
		return exitError
	}
	inv.output.print(stdout, devs)
	if len(devs) > 0 {
		return exitDeviations
	}
	return exitOK
}

func findCommand(args []string) (command, bool) {
	if len(args) == 0 {
		return command{}, false
	}
	i := slices.IndexFunc(commands, func(c command) bool { return c.name == args[0] })
	if i < 0 {
		return command{}, false
	}
	return commands[i], true
}

func wantsHelp(args []string) bool {
	return len(args) > 0 && slices.Contains([]string{"-h", "-help", "--help", "help"}, args[0])
}
