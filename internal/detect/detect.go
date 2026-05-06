package detect

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gumieri/nenyactl/internal/paths"
)

type Mode int

const (
	ModeNone Mode = iota
	ModeBareMetal
	ModeContainer
)

func (m Mode) String() string {
	switch m {
	case ModeBareMetal:
		return "bare-metal"
	case ModeContainer:
		return "container"
	default:
		return "none"
	}
}

type Info struct {
	Mode       Mode
	ConfigFile string
	ConfigD    string
	BinPath    string
	DataDir    string
}

func Detect() (*Info, error) {
	look := func(name string) (string, error) {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
		for _, p := range knownBinPaths() {
			if _, statErr := os.Stat(p); statErr == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("nenya binary not found")
	}
	return DetectWith(look, paths.SystemConfigDir)
}

func DetectWith(look lookPathFn, systemConfigDirFn func() string) (*Info, error) {
	bareMetal, bmErr := detectBareMetal(look, systemConfigDirFn)
	container, ctErr := detectContainer()

	if bareMetal != nil && container != nil {
		return nil, &AmbiguousError{
			BinPath:      bareMetal.BinPath,
			ContainerDir: container.DataDir,
		}
	}

	if bareMetal != nil {
		return bareMetal, nil
	}

	if container != nil {
		return container, nil
	}

	if bmErr != nil {
		var cfgErr *ConfigNotFoundError
		var permErr *PermissionError
		if errors.As(bmErr, &cfgErr) || errors.As(bmErr, &permErr) {
			return nil, bmErr
		}
	}

	return nil, &NotFoundError{
		BareMetalErr: bmErr,
		ContainerErr: ctErr,
	}
}

type lookPathFn func(name string) (string, error)

func detectBareMetal(look lookPathFn, systemConfigDirFn func() string) (*Info, error) {
	binPath, err := look("nenya")
	if err != nil {
		return nil, fmt.Errorf("nenya binary not found in PATH")
	}

	configDir := systemConfigDirFn()
	configFile := filepath.Join(configDir, "config.json")
	configD := filepath.Join(configDir, "config.d")

	info := &Info{
		Mode:       ModeBareMetal,
		ConfigFile: configFile,
		ConfigD:    configD,
		BinPath:    binPath,
	}

	if _, statErr := os.Stat(configFile); statErr != nil {
		if errors.Is(statErr, os.ErrPermission) {
			return nil, &PermissionError{
				Path:    configFile,
				BinPath: binPath,
			}
		}
		return nil, &ConfigNotFoundError{
			ConfigFile: configFile,
			BinPath:    binPath,
		}
	}

	if _, readErr := os.ReadFile(configFile); readErr != nil {
		if errors.Is(readErr, os.ErrPermission) {
			return nil, &PermissionError{
				Path:    configFile,
				BinPath: binPath,
			}
		}
	}

	return info, nil
}

func detectContainer() (*Info, error) {
	containerDir, err := paths.ContainerDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine container directory: %w", err)
	}

	composePath := filepath.Join(containerDir, "compose.yml")
	configDir := filepath.Join(containerDir, "config")
	configFile := filepath.Join(configDir, "config.json")

	composeExists := false
	if _, statErr := os.Stat(composePath); statErr == nil {
		composeExists = true
	}

	configExists := false
	if _, statErr := os.Stat(configFile); statErr == nil {
		configExists = true
	}

	if !composeExists && !configExists {
		return nil, fmt.Errorf("no container deployment found at %s", containerDir)
	}

	info := &Info{
		Mode:       ModeContainer,
		ConfigFile: configFile,
		ConfigD:    containerDir,
		DataDir:    containerDir,
	}

	if configExists {
		if _, readErr := os.ReadFile(configFile); readErr != nil {
			if errors.Is(readErr, os.ErrPermission) {
				return nil, &PermissionError{
					Path:        configFile,
					DataDir:     containerDir,
					IsContainer: true,
				}
			}
		}
	}

	return info, nil
}

func DetectFromDir(dir string, mode Mode) *Info {
	switch mode {
	case ModeBareMetal:
		return &Info{
			Mode:       ModeBareMetal,
			ConfigFile: filepath.Join(dir, "config.json"),
			ConfigD:    filepath.Join(dir, "config.d"),
		}
	case ModeContainer:
		return &Info{
			Mode:       ModeContainer,
			ConfigFile: filepath.Join(dir, "config", "config.json"),
			ConfigD:    dir,
			DataDir:    dir,
		}
	default:
		return &Info{
			Mode:       ModeBareMetal,
			ConfigFile: filepath.Join(dir, "config.json"),
			ConfigD:    filepath.Join(dir, "config.d"),
		}
	}
}

func knownBinPaths() []string {
	ps := []string{paths.SystemBinDir() + "/nenya"}
	if userBin, err := paths.UserBinDir(); err == nil {
		ps = append(ps, userBin+"/nenya")
	}
	return ps
}

type AmbiguousError struct {
	BinPath      string
	ContainerDir string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("multiple nenya installations detected\n  bare-metal binary: %s\n  container data:  %s\n\nUse --dir to specify which installation to configure.", e.BinPath, e.ContainerDir)
}

type NotFoundError struct {
	BareMetalErr error
	ContainerErr error
}

func (e *NotFoundError) Error() string {
	return "nenya installation not detected\n\n  Install with:\n    nenyactl install          # bare-metal (linux/macOS)\n    nenyactl containers setup  # container (podman/docker)\n\n  Or use --dir to specify a configuration directory."
}

type PermissionError struct {
	Path        string
	BinPath     string
	DataDir     string
	IsContainer bool
}

func (e *PermissionError) Error() string {
	if e.IsContainer {
		return fmt.Sprintf("config not readable: %s\n\n  Run with: sudo nenyactl agents --dir %s", e.Path, e.DataDir)
	}
	return fmt.Sprintf("config not readable: %s\n\n  Run with: sudo nenyactl agents", e.Path)
}

func (e *PermissionError) Unwrap() error {
	return os.ErrPermission
}

type ConfigNotFoundError struct {
	ConfigFile string
	BinPath    string
}

func (e *ConfigNotFoundError) Error() string {
	return fmt.Sprintf("nenya binary found at %s but config file missing: %s\n\n  Create config with: sudo nenyactl config init", e.BinPath, e.ConfigFile)
}

func (e *ConfigNotFoundError) Unwrap() error {
	return os.ErrNotExist
}
