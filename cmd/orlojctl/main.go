package main

import (
	"fmt"
	"os"

	"github.com/OrlojHQ/orloj/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(cli.ExitCode(err))
	}
}
