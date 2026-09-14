// Command emberd is the Ember daemon: it serves the OpenAI- and
// Anthropic-compatible APIs, the management API and the console, and it
// governs the engine runner processes. Only --version is implemented yet.
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
	fs := flag.NewFlagSet("emberd", flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println("emberd", version.Get())
		return 0
	}
	fmt.Fprintln(os.Stderr, "emberd: not implemented yet ; try --version")
	return 1
}
