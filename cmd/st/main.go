package main

import (
	"fmt"
	"os"

	"github.com/ttstt/st/internal/cli"
)

func main() {
	app := cli.New(os.Stdout, os.Stderr)
	cmd := app.RootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
