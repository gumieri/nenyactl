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
	owner   = "gumieri"
	repo    = "nenya"
	binDir  = "/usr/local/bin"
	confDir = "/etc/nenya"
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

	// Find binary in extracted archive (goreleaser puts it at the root)
	binaryPath := filepath.Join(extractDir, "nenya")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("nenya binary not found in archive at expected path %s", binaryPath)
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

	if err := copyFile(binaryPath, dest, 0o755); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	fmt.Printf("Installed nenya %s to %s\n", version, dest)

	if !cfg.SkipService {
		if err := installServiceFiles(extractDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to install service files: %v\n", err)
			fmt.Fprintln(os.Stderr, "You can run nenya directly from the command line.")
		}
	}

	return nil
}

func installServiceFiles(extractDir string) error {
	switch runtime.GOOS {
	case "linux":
		return copyFromExtract(extractDir, map[string]string{
			"deploy/nenya.service": "/etc/systemd/system/nenya.service",
			"deploy/nenya.socket":  "/etc/systemd/system/nenya.socket",
		})
	case "darwin":
		return copyFromExtract(extractDir, map[string]string{
			"deploy/nenya.plist": "/Library/LaunchDaemons/com.gumieri.nenya.plist",
		})
	}
	return nil
}

func copyFromExtract(extractDir string, paths map[string]string) error {
	for src, dst := range paths {
		srcPath := filepath.Join(extractDir, src)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			return fmt.Errorf("file not found in archive: %s", src)
		}
		dstDir := filepath.Dir(dst)
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dstDir, err)
		}
		if err := copyFile(srcPath, dst, 0o644); err != nil {
			return fmt.Errorf("copy %s to %s: %w", src, dst, err)
		}
		fmt.Printf("Installed %s\n", dst)
	}
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

func copyFile(src, dst string, mode os.FileMode) error {
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

	return os.Chmod(dst, mode)
}
