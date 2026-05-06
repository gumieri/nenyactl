package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func containTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzW := gzip.NewWriter(&buf)
	tarW := tar.NewWriter(gzW)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if err := tarW.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tarW.Write([]byte(content)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}

	_ = tarW.Close()
	_ = gzW.Close()
	return buf.Bytes()
}

func TestDownloadURL(t *testing.T) {
	t.Run("constructs correct URL for current OS/arch", func(t *testing.T) {
		url := downloadURL("v1.0.0")
		if !strings.Contains(url, "github.com") {
			t.Error("URL should contain github.com")
		}
		if !strings.Contains(url, "gumieri") {
			t.Error("URL should contain owner")
		}
		if !strings.Contains(url, "nenya") {
			t.Error("URL should contain repo name")
		}
		if !strings.Contains(url, "v1.0.0") {
			t.Error("URL should contain version")
		}
		if !strings.Contains(url, runtime.GOOS) {
			t.Error("URL should contain OS")
		}
		if !strings.Contains(url, runtime.GOARCH) {
			t.Error("URL should contain arch")
		}
		if !strings.HasSuffix(url, ".tar.gz") {
			t.Error("URL should end with .tar.gz")
		}
	})

	t.Run("URL format is correct", func(t *testing.T) {
		url := downloadURL("v0.2.0")
		expected := "https://github.com/gumieri/nenya/releases/download/v0.2.0/nenya_" +
			"v0.2.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
		if url != expected {
			t.Errorf("downloadURL() = %q, want %q", url, expected)
		}
	})
}

func TestCopyFile(t *testing.T) {
	t.Run("copies file content and sets permissions", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := t.TempDir()
		src := filepath.Join(srcDir, "source.txt")
		dst := filepath.Join(dstDir, "dest.txt")

		if err := os.WriteFile(src, []byte("hello world"), 0o644); err != nil {
			t.Fatalf("write source: %v", err)
		}

		if err := copyFile(src, dst, 0o755); err != nil {
			t.Fatalf("copyFile() error = %v", err)
		}

		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read dest: %v", err)
		}
		if string(data) != "hello world" {
			t.Errorf("dest content = %q, want %q", string(data), "hello world")
		}

		info, err := os.Stat(dst)
		if err != nil {
			t.Fatalf("stat dest: %v", err)
		}
		if info.Mode()&0o755 != 0o755 {
			t.Errorf("dest permissions = %v, want executable", info.Mode().Perm())
		}
	})

	t.Run("errors on missing source", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "dest.txt")
		err := copyFile("/nonexistent/source.txt", dst, 0o644)
		if err == nil {
			t.Fatal("expected error for missing source, got nil")
		}
	})
}

func TestCopyFromExtract(t *testing.T) {
	t.Run("copies files from extract dir to destinations", func(t *testing.T) {
		extractDir := t.TempDir()
		dstBase := t.TempDir()

		srcFile := filepath.Join(extractDir, "deploy/test.txt")
		if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(srcFile, []byte("content"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		paths := map[string]string{
			"deploy/test.txt": filepath.Join(dstBase, "output.txt"),
		}

		if err := copyFromExtract(extractDir, paths); err != nil {
			t.Fatalf("copyFromExtract() error = %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dstBase, "output.txt"))
		if err != nil {
			t.Fatalf("read output: %v", err)
		}
		if string(data) != "content" {
			t.Errorf("output content = %q, want %q", string(data), "content")
		}
	})

	t.Run("errors on missing source in extract", func(t *testing.T) {
		extractDir := t.TempDir()
		dstBase := t.TempDir()

		paths := map[string]string{
			"deploy/missing.txt": filepath.Join(dstBase, "output.txt"),
		}

		err := copyFromExtract(extractDir, paths)
		if err == nil {
			t.Fatal("expected error for missing source, got nil")
		}
	})
}

func TestDownload(t *testing.T) {
	t.Run("successful download", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("binary-data"))
		}))
		defer server.Close()

		dst := filepath.Join(t.TempDir(), "output.bin")
		if err := download(context.Background(), server.URL, dst); err != nil {
			t.Fatalf("download() error = %v", err)
		}

		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read download: %v", err)
		}
		if string(data) != "binary-data" {
			t.Errorf("download content = %q, want %q", string(data), "binary-data")
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		dst := filepath.Join(t.TempDir(), "output.bin")
		err := download(context.Background(), server.URL, dst)
		if err == nil {
			t.Fatal("expected error for 404, got nil")
		}
	})
}

func TestInstall(t *testing.T) {
	tarGz := containTarGz(t, map[string]string{
		"nenya": "fake-binary-content",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarGz)
	}))
	defer server.Close()

	savedDownloadURL := downloadURL
	downloadURL = func(version string) string { return server.URL + "/download" }
	t.Cleanup(func() { downloadURL = savedDownloadURL })

	tmp := t.TempDir()
	cfg := Config{
		UserInstall: true,
		Version:     "v0.0.0-test",
		SkipService: true,
	}

	t.Setenv("HOME", tmp)

	if err := Install(context.Background(), cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	installedPath := filepath.Join(tmp, ".local", "bin", "nenya")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Fatalf("nenya binary not found at %s", installedPath)
	}
}

func TestInstallWindowsError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	err := Install(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected error on windows")
	}
}

func TestInstallServiceFiles(t *testing.T) {
	t.Run("handles missing deploy directory", func(t *testing.T) {
		extractDir := t.TempDir()
		err := installServiceFiles(extractDir)
		if runtime.GOOS == "linux" && err == nil {
			t.Error("expected error for missing systemd files on linux")
		}
	})
}

func TestDownloadErrorPaths(t *testing.T) {
	t.Run("returns error on canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := download(ctx, "http://example.com", "/dev/null")
		if err == nil {
			t.Fatal("expected error for canceled context, got nil")
		}
	})
}

func TestInstallWithHTTP(t *testing.T) {
	tarGz := containTarGz(t, map[string]string{
		"nenya": "fake-binary-content",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarGz)
	}))
	defer server.Close()

	savedDownloadURL := downloadURL
	downloadURL = func(version string) string { return server.URL + "/download" }
	t.Cleanup(func() { downloadURL = savedDownloadURL })

	tmp := t.TempDir()
	cfg := Config{
		UserInstall: true,
		Version:     "v0.0.0-test",
	}

	// Set HOME to tmp so ~/.local/bin resolves inside tmp
	t.Setenv("HOME", tmp)

	if err := InstallWithHTTP(context.Background(), cfg, server.Client()); err != nil {
		t.Fatalf("InstallWithHTTP() error = %v", err)
	}

	// Should NOT be installed to /usr/local/bin (UserInstall=true, but HOME points to tmp)
	installedPath := filepath.Join(tmp, ".local", "bin", "nenya")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Fatalf("nenya binary not found at %s", installedPath)
	}

	data, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(data) != "fake-binary-content" {
		t.Errorf("installed content = %q, want %q", string(data), "fake-binary-content")
	}
}

func TestInstallWithHTTPWindowsError(t *testing.T) {
	t.Run("returns Windows-specific error", func(t *testing.T) {
		tarGz := containTarGz(t, map[string]string{"nenya": "data"})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(tarGz)
		}))
		defer server.Close()

		if runtime.GOOS != "windows" {
			t.Skip("skipping Windows-only test")
		}

		err := InstallWithHTTP(context.Background(), Config{}, server.Client())
		if err == nil {
			t.Fatal("expected error on Windows")
		}
		if !strings.Contains(err.Error(), "Windows") {
			t.Errorf("expected Windows error, got: %v", err)
		}
	})
}
