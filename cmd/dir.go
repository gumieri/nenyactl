package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/paths"
)

// dirKind describes whether a resolved --dir is a config root (the directory
// that holds config.json and config.d) or a container deployment root (the
// directory that holds compose.yml, config/, and secrets/).
type dirKind int

const (
	dirConfigRoot dirKind = iota
	dirContainerRoot
)

// dirResolution is the single place --dir is interpreted. Every command that
// accepts --dir routes through resolveDir, so the flag has exactly one meaning:
//
//   - --dir points at an existing directory and the layout decides how to read
//     it (a container deployment if it contains config/config.json or
//     compose.yml; otherwise a config root);
//   - with --dir omitted, the command's natural default is used (system config
//     root for bare metal, the default container directory for containers).
type dirResolution struct {
	Path string
	Kind dirKind
	Info *detect.Info
}

// resolveDir interprets --dir for a command. defaultContainer selects the
// container default directory when --dir is empty; otherwise the system config
// root is used.
func resolveDir(dir string, defaultContainer bool) (dirResolution, error) {
	if dir == "" {
		if defaultContainer {
			d, err := paths.ContainerDir()
			if err != nil {
				return dirResolution{}, fmt.Errorf("cannot determine default container directory: %w", err)
			}
			return containerRoot(d), nil
		}
		return configRoot(paths.SystemConfigDir()), nil
	}

	if _, err := os.Stat(dir); err != nil {
		return dirResolution{}, fmt.Errorf("--dir: %w", err)
	}

	info, err := detect.DetectFromDirAuto(dir)
	if err != nil {
		return dirResolution{}, fmt.Errorf("--dir: %w", err)
	}
	if info.Mode == detect.ModeContainer {
		return dirResolution{Path: dir, Kind: dirContainerRoot, Info: info}, nil
	}
	return dirResolution{Path: dir, Kind: dirConfigRoot, Info: info}, nil
}

func configRoot(dir string) dirResolution {
	return dirResolution{
		Path: dir,
		Kind: dirConfigRoot,
		Info: &detect.Info{
			Mode:       detect.ModeBareMetal,
			ConfigFile: filepath.Join(dir, "config.json"),
			ConfigD:    filepath.Join(dir, "config.d"),
		},
	}
}

func containerRoot(dir string) dirResolution {
	return dirResolution{
		Path: dir,
		Kind: dirContainerRoot,
		Info: &detect.Info{
			Mode:       detect.ModeContainer,
			ConfigFile: filepath.Join(dir, "config", "config.json"),
			ConfigD:    filepath.Join(dir, "config", "config.d"),
			DataDir:    dir,
		},
	}
}
