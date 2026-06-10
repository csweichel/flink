package main

import (
	"fmt"
	"os"

	"github.com/csweichel/flink/server/cmd"
)

func main() {
	if err := cmd.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
