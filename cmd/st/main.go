package main

import (
	"fmt"
	"os"

	"github.com/ttstt/st/internal/cli"
)

var version = "dev"

func main() {
	app := cli.New(os.Stdout, os.Stderr)
	cmd := app.RootCommand()
	cmd.Version = version
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
