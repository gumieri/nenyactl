package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

func userDataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return d
		}
		return filepath.Join(home, "AppData", "Local")
	default:
		return filepath.Join(home, ".local", "share")
	}
}

func ContainerDir() (string, error) {
	base := userDataDir()
	if base == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(base, "nenyactl", "nenya"), nil
}

func SystemConfigDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/nenya"
	case "windows":
		if d := os.Getenv("ProgramData"); d != "" {
			return filepath.Join(d, "nenya")
		}
		return filepath.Join(os.Getenv("SystemDrive")+"\\", "ProgramData", "nenya")
	default:
		return "/etc/nenya"
	}
}

func SystemBinDir() string {
	switch runtime.GOOS {
	case "darwin", "linux":
		return "/usr/local/bin"
	default:
		if d := os.Getenv("ProgramFiles"); d != "" {
			return filepath.Join(d, "nenya", "bin")
		}
		return "C:\\Program Files\\nenya\\bin"
	}
}

func UserBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin", "linux":
		return filepath.Join(home, ".local", "bin"), nil
	default:
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "Programs", "nenya", "bin"), nil
		}
		return filepath.Join(home, "AppData", "Local", "Programs", "nenya", "bin"), nil
	}
}
