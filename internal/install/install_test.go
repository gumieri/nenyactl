package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
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
		mode := int64(0o644)
		if name == "nenya" || strings.HasSuffix(name, ".service") || strings.HasSuffix(name, ".socket") {
			mode = 0o755
		}
		hdr := &tar.Header{
			Name: name,
			Mode: mode,
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

// releaseServer serves a synthetic but realistically named release: the archive
// plus checksums.txt (with the real SHA-256) and a cosign bundle stub.
func releaseServer(t *testing.T, archive []byte, tag string) *httptest.Server {
	t.Helper()
	name := archiveFilename(tag, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt.sigstore.json"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, "%x  %s\n", sum, name)
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func pointAtServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	saved := assetBaseURL
	assetBaseURL = func(string) string { return server.URL + "/" }
	t.Cleanup(func() { assetBaseURL = saved })
}

// fakeRunner is a CommandRunner for tests. It fails unless the command is
// allowlisted, so `cosign version` can be simulated.
type fakeRunner struct {
	available map[string]bool
}

func (f fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	if f.available[key] {
		return []byte("ok"), nil
	}
	return nil, fmt.Errorf("command not found: %s", key)
}

func TestArchiveFilename(t *testing.T) {
	cases := []struct {
		tag, os, arch, want string
	}{
		{"v0.15.0", "linux", "arm64", "nenya_0.15.0_linux_arm64.tar.gz"},
		{"v0.15.0", "darwin", "amd64", "nenya_0.15.0_darwin_amd64.tar.gz"},
		{"0.15.0", "linux", "amd64", "nenya_0.15.0_linux_amd64.tar.gz"},
	}
	for _, c := range cases {
		if got := archiveFilename(c.tag, c.os, c.arch); got != c.want {
			t.Errorf("archiveFilename(%q,%q,%q) = %q, want %q", c.tag, c.os, c.arch, got, c.want)
		}
	}
}

func TestDownloadURL(t *testing.T) {
	tag := "v0.2.0"
	url := downloadURL(tag)
	want := "https://github.com/gumieri/nenya/releases/download/v0.2.0/nenya_0.2.0_" +
		runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	if url != want {
		t.Errorf("downloadURL() = %q, want %q", url, want)
	}
	if got := checksumsURL(tag); !strings.HasSuffix(got, "/v0.2.0/checksums.txt") {
		t.Errorf("checksumsURL() = %q", got)
	}
	if got := sigstoreBundleURL(tag); !strings.HasSuffix(got, "/v0.2.0/checksums.txt.sigstore.json") {
		t.Errorf("sigstoreBundleURL() = %q", got)
	}
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
	archive := containTarGz(t, map[string]string{
		"nenya":                "fake-binary-content",
		"deploy/nenya.service": "[Unit]\nDescription=nenya",
		"deploy/nenya.socket":  "[Socket]\nListenStream=8080",
	})
	server := releaseServer(t, archive, "v0.0.0-test")
	defer server.Close()
	pointAtServer(t, server)

	tmp := t.TempDir()
	cfg := Config{
		UserInstall: true,
		Version:     "v0.0.0-test",
		SkipService: true,
		SkipVerify:  true,
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
	archive := containTarGz(t, map[string]string{
		"nenya": "fake-binary-content",
	})
	server := releaseServer(t, archive, "v0.0.0-test")
	defer server.Close()
	pointAtServer(t, server)

	tmp := t.TempDir()
	cfg := Config{
		UserInstall: true,
		Version:     "v0.0.0-test",
		SkipVerify:  true,
	}
	t.Setenv("HOME", tmp)

	if err := InstallWithHTTP(context.Background(), cfg, server.Client()); err != nil {
		t.Fatalf("InstallWithHTTP() error = %v", err)
	}

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
		if runtime.GOOS != "windows" {
			t.Skip("skipping Windows-only test")
		}
		archive := containTarGz(t, map[string]string{"nenya": "data"})
		server := releaseServer(t, archive, "v0.0.0")
		defer server.Close()
		pointAtServer(t, server)

		err := InstallWithHTTPAndRunner(context.Background(), Config{}, server.Client(), fakeRunner{})
		if err == nil {
			t.Fatal("expected error on Windows")
		}
		if !strings.Contains(err.Error(), "Windows") {
			t.Errorf("expected Windows error, got: %v", err)
		}
	})
}
