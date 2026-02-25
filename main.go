package main

import (
	"os"

	"github.com/podspawn/podspawn/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
