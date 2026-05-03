package install

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

const (
	owner    = "gumieri"
	repo     = "nenya"
	binDir   = "/usr/local/bin"
	confDir  = "/etc/nenya"
)

type Config struct {
	UserInstall bool
	Version     string
	SkipService bool
}

func Install(ctx context.Context, cfg Config) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("bare-metal installation is not supported on Windows; use 'nenyactl containers setup' instead")
	}

	version := cfg.Version
	if version == "" {
		var err error
		version, err = FetchLatestVersion(ctx)
		if err != nil {
			return fmt.Errorf("fetch latest version: %w", err)
		}
	}

	url := downloadURL(version)
	tmpDir, err := os.MkdirTemp("", "nenyactl-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archive := filepath.Join(tmpDir, "nenya.tar.gz")
	if err := download(ctx, url, archive); err != nil {
		return fmt.Errorf("download nenya: %w", err)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return fmt.Errorf("create extract dir: %w", err)
	}

	if err := untar(archive, extractDir); err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}

	var binaryPath string
	found := false
	filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Mode().IsRegular() && info.Name() == "nenya" {
			binaryPath = path
			found = true
		}
		return nil
	})

	if !found {
		return fmt.Errorf("nenya binary not found in archive")
	}

	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return fmt.Errorf("set executable: %w", err)
	}

	dest := filepath.Join(binDir, "nenya")
	if cfg.UserInstall {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		dest = filepath.Join(home, ".local", "bin", "nenya")
	}

	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	if err := copyFile(binaryPath, dest); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	fmt.Printf("Installed nenya %s to %s\n", version, dest)

	if !cfg.SkipService {
		if err := installService(ctx, dest); err != nil {
			fmt.Printf("Warning: failed to install service: %v\n", err)
			fmt.Println("You can run nenya directly from the command line.")
		}
	}

	return nil
}

func installService(ctx context.Context, binaryPath string) error {
	switch runtime.GOOS {
	case "linux":
		return installSystemd(binaryPath)
	case "darwin":
		return installLaunchd(binaryPath)
	}
	return nil
}

func installSystemd(binaryPath string) error {
	systemdDir := "/etc/systemd/system"

	if err := os.MkdirAll(systemdDir, 0o755); err != nil {
		return fmt.Errorf("create systemd dir: %w", err)
	}

	servicePath := filepath.Join(systemdDir, "nenya.service")
	if err := os.WriteFile(servicePath, []byte(SystemdService), 0o644); err != nil {
		return fmt.Errorf("write service unit: %w", err)
	}

	socketPath := filepath.Join(systemdDir, "nenya.socket")
	if err := os.WriteFile(socketPath, []byte(SystemdSocket), 0o644); err != nil {
		return fmt.Errorf("write socket unit: %w", err)
	}

	fmt.Printf("Installed systemd units to %s/\n", systemdDir)
	fmt.Println("To enable and start: sudo systemctl enable --now nenya")
	return nil
}

func installLaunchd(binaryPath string) error {
	launchdDir := "/Library/LaunchDaemons"
	plistPath := filepath.Join(launchdDir, "com.gumieri.nenya.plist")

	if err := os.MkdirAll(launchdDir, 0o755); err != nil {
		return fmt.Errorf("create launchd dir: %w", err)
	}

	if err := os.WriteFile(plistPath, []byte(LaunchdPlist), 0o644); err != nil {
		return fmt.Errorf("write launchd plist: %w", err)
	}

	fmt.Printf("Installed launchd plist to %s\n", plistPath)
	fmt.Println("To enable and start: sudo launchctl load -w /Library/LaunchDaemons/com.gumieri.nenya.plist")
	return nil
}

func downloadURL(version string) string {
	arch := runtime.GOARCH
	osName := runtime.GOOS
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/nenya_%s_%s_%s.tar.gz",
		owner, repo, version, version, osName, arch)
}

func download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s for %s", resp.Status, url)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	if _, err := io.Copy(d, s); err != nil {
		return err
	}

	return os.Chmod(dst, 0o755)
}