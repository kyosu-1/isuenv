package main

import (
	"os"

	"github.com/kyosu-1/isuenv/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
