package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Tests for the platform-branching directory helpers (NCTL-50): the For
// variants carry GOOS and environment as parameters, so every branch is
// exercised on any host. The home-resolution error is propagated verbatim,
// pinned with errors.Is.

func TestUserDataDirFor(t *testing.T) {
	cases := []struct {
		name         string
		goos         string
		localAppData string
		home         string
		want         string
	}{
		{"linux ignores LOCALAPPDATA", "linux", "/lad", "/home/u", "/home/u/.local/share"},
		{"darwin ignores LOCALAPPDATA", "darwin", "/lad", "/Users/u", "/Users/u/Library/Application Support"},
		{"windows prefers LOCALAPPDATA", "windows", `C:\lad`, `C:\Users\u`, `C:\lad`},
		{"windows falls back to home", "windows", "", `C:\Users\u`, filepath.Join(`C:\Users\u`, "AppData", "Local")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := userDataDirFor(tc.goos, tc.localAppData, tc.home); got != tc.want {
				t.Errorf("userDataDirFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestContainerDirFor(t *testing.T) {
	t.Run("empty base errors with the sentinel", func(t *testing.T) {
		if _, err := containerDirFor(""); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("err = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("joins the data root", func(t *testing.T) {
		got, err := containerDirFor("/data")
		if err != nil {
			t.Fatal(err)
		}
		if want := "/data/nenyactl/nenya"; got != want {
			t.Errorf("containerDirFor = %q, want %q", got, want)
		}
	})
}

func TestUserConfigDirFor(t *testing.T) {
	cases := []struct {
		name    string
		goos    string
		xdg     string
		appData string
		home    string
		want    string
	}{
		{"xdg wins on linux", "linux", "/xdg", "", "/home/u", "/xdg/nenya"},
		{"xdg wins on windows", "windows", `C:\xdg`, `C:\ad`, `C:\Users\u`, filepath.Join(`C:\xdg`, "nenya")},
		{"linux default", "linux", "", "", "/home/u", "/home/u/.config/nenya"},
		{"darwin default", "darwin", "", "", "/Users/u", "/Users/u/Library/Application Support/nenya"},
		{"windows prefers APPDATA", "windows", "", `C:\ad`, `C:\Users\u`, filepath.Join(`C:\ad`, "nenya")},
		{"windows falls back to home", "windows", "", "", `C:\Users\u`, filepath.Join(`C:\Users\u`, "AppData", "Roaming", "nenya")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := userConfigDirFor(tc.goos, tc.xdg, tc.appData, tc.home, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("userConfigDirFor = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("propagates the home-resolution error", func(t *testing.T) {
		homeErr := errors.New("$HOME is not defined")
		_, err := userConfigDirFor("linux", "", "", "", homeErr)
		if !errors.Is(err, homeErr) {
			t.Errorf("err = %v, want the original home error", err)
		}
	})
}

func TestSystemConfigDirFor(t *testing.T) {
	cases := []struct {
		name        string
		goos        string
		programData string
		systemDrive string
		want        string
	}{
		{"darwin", "darwin", "", "", "/Library/Application Support/nenya"},
		{"windows with ProgramData", "windows", `C:\ProgramData`, "C:", filepath.Join(`C:\ProgramData`, "nenya")},
		{"windows falls back to SystemDrive", "windows", "", "D:", filepath.Join("D:\\", "ProgramData", "nenya")},
		{"windows empty SystemDrive defaults to C:", "windows", "", "", filepath.Join("C:\\", "ProgramData", "nenya")},
		{"linux", "linux", "", "", "/etc/nenya"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := systemConfigDirFor(tc.goos, tc.programData, tc.systemDrive); got != tc.want {
				t.Errorf("systemConfigDirFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSystemBinDirFor(t *testing.T) {
	cases := []struct {
		name        string
		goos        string
		programData string
		want        string
	}{
		{"linux", "linux", "", "/usr/bin"},
		{"darwin", "darwin", "", "/usr/bin"},
		{"windows with ProgramFiles", "windows", `C:\PF`, filepath.Join(`C:\PF`, "nenya", "bin")},
		{"windows fallback", "windows", "", `C:\Program Files\nenya\bin`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := systemBinDirFor(tc.goos, tc.programData); got != tc.want {
				t.Errorf("systemBinDirFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUserBinDirFor(t *testing.T) {
	cases := []struct {
		name         string
		goos         string
		localAppData string
		home         string
		want         string
	}{
		{"linux", "linux", "", "/home/u", "/home/u/.local/bin"},
		{"darwin", "darwin", "", "/Users/u", "/Users/u/.local/bin"},
		{"windows prefers LOCALAPPDATA", "windows", `C:\lad`, `C:\Users\u`, filepath.Join(`C:\lad`, "Programs", "nenya", "bin")},
		{"windows falls back to home", "windows", "", `C:\Users\u`, filepath.Join(`C:\Users\u`, "AppData", "Local", "Programs", "nenya", "bin")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := userBinDirFor(tc.goos, tc.localAppData, tc.home, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("userBinDirFor = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("propagates the home-resolution error", func(t *testing.T) {
		homeErr := errors.New("$HOME is not defined")
		_, err := userBinDirFor("linux", "", "", homeErr)
		if !errors.Is(err, homeErr) {
			t.Errorf("err = %v, want the original home error", err)
		}
	})
}

func TestOSNativeShapes(t *testing.T) {
	// The exported wrappers must wire runtime.GOOS and the real environment
	// through: pin them to the running platform's default branch.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("ProgramData", "")
	t.Setenv("SystemDrive", "")
	t.Setenv("ProgramFiles", "")
	t.Setenv("LOCALAPPDATA", "")

	switch runtime.GOOS {
	case "darwin":
		if got := SystemConfigDir(); got != "/Library/Application Support/nenya" {
			t.Errorf("SystemConfigDir = %q, want the darwin default", got)
		}
		if got := SystemBinDir(); got != "/usr/bin" {
			t.Errorf("SystemBinDir = %q, want /usr/bin", got)
		}
	case "windows":
		if got := SystemConfigDir(); got != `C:\ProgramData\nenya` {
			t.Errorf("SystemConfigDir = %q, want the C: fallback", got)
		}
		if got := SystemBinDir(); got != `C:\Program Files\nenya\bin` {
			t.Errorf("SystemBinDir = %q, want the ProgramFiles fallback", got)
		}
	default:
		if got := SystemConfigDir(); got != "/etc/nenya" {
			t.Errorf("SystemConfigDir = %q, want /etc/nenya", got)
		}
		if got := SystemBinDir(); got != "/usr/bin" {
			t.Errorf("SystemBinDir = %q, want /usr/bin", got)
		}
	}
}
