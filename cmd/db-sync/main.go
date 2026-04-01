package main

import (
	"fmt"
	"os"

	"db-sync/internal/cli"
)

func main() {
	app := cli.NewApp(os.Stdout, os.Stderr)
	root := cli.NewRootCommand(app)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
