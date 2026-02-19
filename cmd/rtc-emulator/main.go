package main

import (
	"fmt"
	"os"

	"github.com/supurazako/rtc-emulator/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
