package main

import (
	"os"
	"runtime/debug"

	"github.com/gumieri/nenyactl/cmd"
	"github.com/gumieri/nenyactl/internal/version"
)

func main() {
	if version.Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					version.Version = setting.Value[:8]
					version.Commit = setting.Value
					break
				}
			}
		}
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
