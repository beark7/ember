// Command ember is the Ember command-line client of emberd (serve, run, models,
// plan, doctor, keys, usage, config, node, fleet). Only --version is
// implemented yet.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/beark7/ember/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("ember", flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("ember", version.Get())
		return 0
	}
	fmt.Fprintln(os.Stderr, "ember: not implemented yet ; try --version")
	return 1
}
