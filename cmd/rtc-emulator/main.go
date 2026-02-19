package main

import (
	"os"

	"github.com/supurazako/rtc-emulator/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
