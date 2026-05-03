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
}

func Install(ctx context.Context, cfg Config) error {
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