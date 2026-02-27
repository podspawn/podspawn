package main

import (
	"os"

	"github.com/podspawn/podspawn/cmd"
)

func main() {
	err := cmd.Execute()
	cmd.CloseLog()
	if err != nil {
		os.Exit(1)
	}
}
