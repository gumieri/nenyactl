package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/gumieri/nenyactl/internal/paths"
)

// dirMode selects how a command treats --dir.
type dirMode int

const (
	// dirAttach requires --dir to exist (read/modify commands).
	dirAttach dirMode = iota
	// dirCreate allows --dir to be created by the command (init/setup/bootstrap).
	dirCreate
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
// a deployment root whose layout determines how it is read.
type dirResolution struct {
	// Path is the directory the user pointed at, or the command's default.
	Path string
	// Kind records whether Path is a bare-metal config root or a container
	// deployment root.
	Kind dirKind
	// Info holds the resolved config/secrets/unit paths for the deployment.
	Info *detect.Info
	// cpaths holds the contract-resolved filesystem layout from
	// `nenya paths --json`, or nil when it could not be resolved (container
	// deployments, or a nenya without the command — see refineWithPaths).
	cpaths *nenya.Paths
}

// ContractPaths returns the deployment's resolved filesystem contract from
// `nenya paths --json` (CONTRACT.md §4.2), or nil when it could not be
// resolved. Consumers MUST treat nil as "unknown", never as a reason to
// reconstruct the resolution themselves.
func (r dirResolution) ContractPaths() *nenya.Paths { return r.cpaths }

// pathsProbe resolves the deployment's filesystem contract through
// `nenya paths --json`. A non-empty dir is passed as --config-dir; an empty
// dir means no target flags, so nenya applies §3.3 itself (NENYA_CONFIG_DIR /
// NENYA_CONFIG_FILE, then the platform default). Package var so tests can
// inject a fake without a nenya binary.
var pathsProbe = func(dir string) (*nenya.Paths, error) {
	p, err := newContractClient(dir).Paths(context.Background())
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// refineWithPaths overlays `nenya paths --json` onto a resolved bare-metal
// config root. nenya resolves the same flags and environment as the server
// (§4.2), so the contract answer wins over the local structural layout: it
// honors NENYA_CONFIG_DIR / NENYA_CONFIG_FILE and tracks nenya's resolution
// without nenyactl re-deriving it (the boundary rule). A failed probe — binary
// absent, or a released nenya without the command — keeps the local layout,
// the documented shim; for the default root it says so on stderr rather than
// silently ignoring the §3.3 environment. An explicit --dir needs no note: the
// local layout for that directory is what paths --config-dir would report.
func refineWithPaths(res dirResolution, dir string) dirResolution {
	if res.Kind != dirConfigRoot {
		return res
	}
	p, err := pathsProbe(dir)
	if err != nil || p == nil || p.ConfigDir == "" || p.ConfigFile == "" {
		if dir == "" {
			fmt.Fprintln(os.Stderr, dimStyle.Render("  nenya paths --json unavailable; assuming the default config root"))
		}
		return res
	}
	res.Path = p.ConfigDir
	res.Info.ConfigFile = p.ConfigFile
	res.Info.ConfigD = p.ConfigD
	res.cpaths = p
	return res
}

// resolveDir interprets --dir for a command.
//
//   - dirAttach requires the path to exist; a missing path is an error.
//   - dirCreate accepts a missing path so create-oriented commands can make it.
//
// A container layout (config/config.json or compose.yml) resolves to the
// nested container paths; anything else is treated as a bare-metal config root
// and refined through `nenya paths --json`. An empty --dir uses the server's
// own resolution (§3.3 environment) via the same command, falling back to the
// system config root.
func resolveDir(dir string, mode dirMode, defaultContainer bool) (dirResolution, error) {
	if dir == "" {
		if defaultContainer {
			d, err := paths.ContainerDir()
			if err != nil {
				return dirResolution{}, fmt.Errorf("cannot determine default container directory: %w", err)
			}
			return containerRoot(d), nil
		}
		return refineWithPaths(configRoot(paths.SystemConfigDir()), ""), nil
	}

	if mode == dirAttach {
		if _, err := os.Stat(dir); err != nil {
			return dirResolution{}, fmt.Errorf("--dir: %w", err)
		}
	}

	if detect.ModeForDir(dir) == detect.ModeContainer {
		return containerRoot(dir), nil
	}
	return refineWithPaths(configRoot(dir), dir), nil
}

// ConfigDir is the directory that holds config.json/config.d for the resolved
// deployment. Callers that write config must use this, never Path.
func (r dirResolution) ConfigDir() string {
	return filepath.Dir(r.Info.ConfigFile)
}

// Contract returns a nenya contract client pinned to the resolved config
// directory, so every call (paths/describe/config set) targets the same root.
func (r dirResolution) Contract() *nenya.Client {
	return newContractClient(r.ConfigDir())
}

// newContractClient builds the nenya contract client for a config directory. It
// is a package variable so tests can inject a fake Runner and exercise the
// command wiring without a nenya binary installed. It is not safe for parallel
// tests: tests swap it and must run sequentially.
var newContractClient = func(dir string) *nenya.Client {
	return nenya.New(nenya.Binary{}).WithConfigDir(dir)
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

// detectedResolution converts an auto-detected install into a dirResolution.
// The deployment root is the container data dir or the config file's directory,
// never the config file itself. A bare-metal detection is refined through
// `nenya paths --json` (the server's §3.3 resolution is authoritative for the
// machine's install); containers are resolved locally.
func detectedResolution(info *detect.Info) dirResolution {
	root := filepath.Dir(info.ConfigFile)
	kind := dirConfigRoot
	if info.Mode == detect.ModeContainer {
		kind = dirContainerRoot
		if info.DataDir != "" {
			root = info.DataDir
		}
	}
	res := dirResolution{Path: root, Kind: kind, Info: info}
	return refineWithPaths(res, "")
}
