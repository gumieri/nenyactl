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

// SecretsDir returns the directory the deployment's secrets live in: the
// container merge directory (mounted at /run/secrets/nenya by the generated
// compose) or the bare-metal config root. It is the directory nenyactl targets
// `nenya secret set` at and scans for existing secrets; the effective
// *location* a token resolves from is the contract's answer (`nenya secret
// get`, `paths.secrets_file`, `describe.secrets.active_source`), not this.
func (i *Info) SecretsDir() string {
	if i.Mode == ModeContainer {
		if i.DataDir == "" {
			return ""
		}
		return filepath.Join(i.DataDir, "secrets")
	}
	if i.ConfigFile == "" {
		return ""
	}
	return filepath.Dir(i.ConfigFile)
}

func Detect() (*Info, error) {
	look := func(name string) (string, error) {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
		for _, p := range knownBinPaths() {
			if info, statErr := os.Stat(p); statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
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

	if _, readErr := os.ReadFile(configFile); readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return nil, &ConfigNotFoundError{
				ConfigFile: configFile,
				BinPath:    binPath,
			}
		}
		if errors.Is(readErr, os.ErrPermission) {
			return nil, &PermissionError{
				Path:    configFile,
				BinPath: binPath,
			}
		}
		return nil, fmt.Errorf("read config: %w", readErr)
	}

	return info, nil
}

func detectContainer() (*Info, error) {
	containerDir, err := paths.ContainerDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine container directory: %w", err)
	}

	composePath := filepath.Join(containerDir, "compose.yml")
	configFile := filepath.Join(containerDir, "config", "config.json")

	_, composeStatErr := os.Stat(composePath)
	_, configStatErr := os.Stat(configFile)

	composeExists := composeStatErr == nil
	configExists := configStatErr == nil

	if !composeExists && !configExists {
		return nil, fmt.Errorf("no container deployment found at %s", containerDir)
	}

	info := &Info{
		Mode:       ModeContainer,
		ConfigFile: configFile,
		ConfigD:    filepath.Join(containerDir, "config", "config.d"),
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
			return nil, fmt.Errorf("read container config: %w", readErr)
		}
	}

	return info, nil
}

func DetectFromDir(dir string, mode Mode) (*Info, error) {
	switch mode {
	case ModeBareMetal:
		return &Info{
			Mode:       ModeBareMetal,
			ConfigFile: filepath.Join(dir, "config.json"),
			ConfigD:    filepath.Join(dir, "config.d"),
		}, nil
	case ModeContainer:
		return &Info{
			Mode:       ModeContainer,
			ConfigFile: filepath.Join(dir, "config", "config.json"),
			ConfigD:    filepath.Join(dir, "config", "config.d"),
			DataDir:    dir,
		}, nil
	default:
		return nil, fmt.Errorf("invalid mode: %d", mode)
	}
}

// ModeForDir infers the installation mode from the layout inside dir. A
// container deployment has config/config.json (and/or compose.yml); anything
// else is treated as bare-metal. This is what lets `agents --dir` resolve the
// right config path without the caller knowing the layout.
func ModeForDir(dir string) Mode {
	if _, err := os.Stat(filepath.Join(dir, "config", "config.json")); err == nil {
		return ModeContainer
	}
	if _, err := os.Stat(filepath.Join(dir, "compose.yml")); err == nil {
		return ModeContainer
	}
	return ModeBareMetal
}

func knownBinPaths() []string {
	ps := []string{filepath.Join(paths.SystemBinDir(), "nenya")}
	if userBin, err := paths.UserBinDir(); err == nil {
		ps = append(ps, filepath.Join(userBin, "nenya"))
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
		return fmt.Sprintf("config not readable: %s\n\n  This is a container deployment under %s. Check the file permissions and ownership for your user account (no sudo needed for a user-owned deployment).", e.Path, e.DataDir)
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
