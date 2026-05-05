package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func createTarGz(t *testing.T, files map[string]string) string {
	t.Helper()

	var buf bytes.Buffer
	gzW := gzip.NewWriter(&buf)
	tarW := tar.NewWriter(gzW)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tarW.WriteHeader(hdr); err != nil {
			t.Fatalf("tar write header: %v", err)
		}
		if _, err := tarW.Write([]byte(content)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}

	if err := tarW.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gzW.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tar.gz: %v", err)
	}
	return path
}

func TestUntar(t *testing.T) {
	t.Run("extracts regular files", func(t *testing.T) {
		src := createTarGz(t, map[string]string{
			"nenya":         "binary-content",
			"deploy/service": "[Unit]\nDescription=nenya",
		})
		dst := t.TempDir()

		if err := untar(src, dst); err != nil {
			t.Fatalf("untar() error = %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dst, "nenya"))
		if err != nil {
			t.Fatalf("read extracted nenya: %v", err)
		}
		if string(data) != "binary-content" {
			t.Errorf("extracted content = %q, want %q", string(data), "binary-content")
		}

		svcData, err := os.ReadFile(filepath.Join(dst, "deploy/service"))
		if err != nil {
			t.Fatalf("read extracted deploy/service: %v", err)
		}
		if string(svcData) != "[Unit]\nDescription=nenya" {
			t.Errorf("unexpected service file content: %q", string(svcData))
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		src := createTarGz(t, map[string]string{
			"../../etc/passwd": "malicious",
		})
		dst := t.TempDir()

		err := untar(src, dst)
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
	})

	t.Run("handles corrupt gzip", func(t *testing.T) {
		tmp := t.TempDir()
		badPath := filepath.Join(tmp, "corrupt.tar.gz")
		if err := os.WriteFile(badPath, []byte("not-a-tar-gz"), 0o644); err != nil {
			t.Fatalf("write corrupt file: %v", err)
		}

		err := untar(badPath, tmp)
		if err == nil {
			t.Fatal("expected error for corrupt gzip, got nil")
		}
	})

	t.Run("handles missing source", func(t *testing.T) {
		dst := t.TempDir()
		err := untar("/nonexistent/path.tar.gz", dst)
		if err == nil {
			t.Fatal("expected error for missing source, got nil")
		}
	})
}
