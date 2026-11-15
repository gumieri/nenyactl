package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// ContainerDir resolves the per-user container deployment root:
// $XDG_DATA_HOME/nenyactl/nenya, falling back to the platform per-user data
// dir (LOCALAPPDATA on Windows). It errors when no data root can be resolved.
func ContainerDir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return containerDirFor(d)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return containerDirFor(userDataDirFor(runtime.GOOS, os.Getenv("LOCALAPPDATA"), home))
}

// containerDirFor joins the resolved data root; the empty-base sentinel
// (os.ErrNotExist) is unreachable through ContainerDir today but is the
// documented contract for future callers.
func containerDirFor(base string) (string, error) {
	if base == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(base, "nenyactl", "nenya"), nil
}

// userDataDirFor is ContainerDir's platform dispatch, parameterized so every
// branch is testable on any host (NCTL-50): LOCALAPPDATA wins on Windows,
// then the home-derived default.
func userDataDirFor(goos, localAppData, home string) string {
	if goos == "windows" && localAppData != "" {
		return localAppData
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	case "windows":
		return filepath.Join(home, "AppData", "Local")
	default:
		return filepath.Join(home, ".local", "share")
	}
}

// UserConfigDir returns the per-user config directory for nenya
// ($XDG_CONFIG_HOME/nenya on Linux, Application Support on macOS,
// %APPDATA%\nenya on Windows). It propagates the home-resolution error so
// callers can distinguish it from a resolved-but-absent path.
func UserConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	return userConfigDirFor(runtime.GOOS, os.Getenv("XDG_CONFIG_HOME"), os.Getenv("APPDATA"), home, err)
}

func userConfigDirFor(goos, xdg, appData, home string, homeErr error) (string, error) {
	if xdg != "" {
		return filepath.Join(xdg, "nenya"), nil
	}
	if homeErr != nil {
		return "", homeErr
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "nenya"), nil
	case "windows":
		if appData != "" {
			return filepath.Join(appData, "nenya"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "nenya"), nil
	default:
		return filepath.Join(home, ".config", "nenya"), nil
	}
}

func SystemConfigDir() string {
	return systemConfigDirFor(runtime.GOOS, os.Getenv("ProgramData"), os.Getenv("SystemDrive"))
}

func systemConfigDirFor(goos, programData, systemDrive string) string {
	switch goos {
	case "darwin":
		return "/Library/Application Support/nenya"
	case "windows":
		if programData != "" {
			return filepath.Join(programData, "nenya")
		}
		if systemDrive == "" {
			systemDrive = `C:`
		}
		return filepath.Join(systemDrive+`\`, "ProgramData", "nenya")
	default:
		return "/etc/nenya"
	}
}

func SystemBinDir() string {
	return systemBinDirFor(runtime.GOOS, os.Getenv("ProgramFiles"))
}

func systemBinDirFor(goos, programFiles string) string {
	switch goos {
	case "darwin", "linux":
		return "/usr/bin"
	default:
		if programFiles != "" {
			return filepath.Join(programFiles, "nenya", "bin")
		}
		return `C:\Program Files\nenya\bin`
	}
}

func UserBinDir() (string, error) {
	home, err := os.UserHomeDir()
	return userBinDirFor(runtime.GOOS, os.Getenv("LOCALAPPDATA"), home, err)
}

func userBinDirFor(goos, localAppData, home string, homeErr error) (string, error) {
	if homeErr != nil {
		return "", homeErr
	}
	switch goos {
	case "darwin", "linux":
		return filepath.Join(home, ".local", "bin"), nil
	default:
		if localAppData != "" {
			return filepath.Join(localAppData, "Programs", "nenya", "bin"), nil
		}
		return filepath.Join(home, "AppData", "Local", "Programs", "nenya", "bin"), nil
	}
}
