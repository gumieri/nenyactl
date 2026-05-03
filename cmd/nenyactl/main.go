package main

import (
	"os"
	"runtime/debug"

	"github.com/gumieri/nenyactl/cmd"
)

var version = "0.1.0"

func main() {
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				version = setting.Value[:8]
				break
			}
		}
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}