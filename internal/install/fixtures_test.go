package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type archiveManifest struct {
	Description string `json:"description"`
	Source      string `json:"source"`
	Release     string `json:"release"`
	Linux       struct {
		Archive string   `json:"archive"`
		Members []string `json:"members"`
	} `json:"linux"`
	Darwin struct {
		Archive string   `json:"archive"`
		Members []string `json:"members"`
	} `json:"darwin"`
}

func readManifest(t *testing.T) archiveManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "archive", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m archiveManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m
}

func membersFor(m archiveManifest) []string {
	if runtime.GOOS == "darwin" {
		return m.Darwin.Members
	}
	return m.Linux.Members
}

// tarGzFromTestdata builds an archive from the checked-in real-layout files.
func tarGzFromTestdata(t *testing.T, members []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range members {
		// The binary member is large; only its name and mode matter here. The
		// unit members are the real checked-in files.
		content := []byte("#!/bin/sh\necho placeholder\n")
		if name != "nenya" {
			var err error
			content, err = os.ReadFile(filepath.Join("testdata", "archive", filepath.FromSlash(name)))
			if err != nil {
				t.Fatalf("read fixture %s: %v", name, err)
			}
		}
		mode := int64(0o644)
		if name == "nenya" {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(content))}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// TestRealArchiveFixtures asserts the checked-in fixtures match CONTRACT.md
// §7.1 and that the installer extracts the real unit members intact.
func TestRealArchiveFixtures(t *testing.T) {
	m := readManifest(t)
	cases := []struct {
		osName string
		got    []string
		want   []string
	}{
		{"linux", m.Linux.Members, []string{"deploy/nenya.service", "deploy/nenya.socket", "nenya"}},
		{"darwin", m.Darwin.Members, []string{"deploy/nenya.plist", "nenya"}},
	}
	for _, c := range cases {
		if !sameMembers(c.got, c.want) {
			t.Errorf("%s manifest members = %v, want %v", c.osName, c.got, c.want)
		}
	}

	if runtime.GOOS == "windows" {
		t.Skip("no install service files on windows")
	}

	archive := tarGzFromTestdata(t, membersFor(m))
	src := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(src, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	extractDir := filepath.Join(t.TempDir(), "extract")
	if err := untar(src, extractDir); err != nil {
		t.Fatalf("untar real-layout fixture: %v", err)
	}

	unitDir := filepath.Join(t.TempDir(), "units")
	if err := installServiceFilesTo(extractDir, unitDir); err != nil {
		t.Fatalf("installServiceFilesTo: %v", err)
	}

	expect := map[string]string{}
	if runtime.GOOS == "darwin" {
		expect["com.gumieri.nenya.plist"] = "deploy/nenya.plist"
	} else {
		expect["nenya.service"] = "deploy/nenya.service"
		expect["nenya.socket"] = "deploy/nenya.socket"
	}
	for dstName, srcName := range expect {
		got, err := os.ReadFile(filepath.Join(unitDir, dstName))
		if err != nil {
			t.Fatalf("read installed %s: %v", dstName, err)
		}
		wantContent, err := os.ReadFile(filepath.Join("testdata", "archive", filepath.FromSlash(srcName)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, wantContent) {
			t.Errorf("installed %s does not match the real unit content", dstName)
		}
	}
}

func sameMembers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]int{}
	for _, s := range a {
		set[s]++
	}
	for _, s := range b {
		set[s]--
	}
	for _, v := range set {
		if v != 0 {
			return false
		}
	}
	return true
}
