package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"contracter/internal/contract"
)

// printer is a way to print deviations.
type printer struct {
	print func(io.Writer, []contract.Deviation)
	// document is set when the output is for another program to parse. It
	// must then be alone on stdout, so everything else goes to stderr.
	document bool
}

var printers = map[string]printer{
	"text": {print: printText},
	"json": {print: printJSON, document: true},
}

func printText(w io.Writer, devs []contract.Deviation) {
	if len(devs) == 0 {
		return
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for _, d := range devs {
		fmt.Fprintf(table, "%s\t%s\t%s\n", d.File, d.Key, d.Msg)
	}
	table.Flush()
	fmt.Fprintf(w, "%d deviation(s)\n", len(devs))
}

func printJSON(w io.Writer, devs []contract.Deviation) {
	if devs == nil {
		devs = []contract.Deviation{} // "deviations": [] rather than null
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	enc.Encode(map[string][]contract.Deviation{"deviations": devs})
}

func usage(w io.Writer, fl *flag.FlagSet) {
	fmt.Fprint(w, "usage: contracter <command> [flags]\n\n")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
	fmt.Fprint(w, "\nflags:\n")
	fl.PrintDefaults()
	fmt.Fprintf(w, "\nexit status: %d ok, %d deviations found, %d error\n", exitOK, exitDeviations, exitError)
}
