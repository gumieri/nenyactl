package install

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWarnIfNonDefaultRoot(t *testing.T) {
	t.Run("notes when the unit was generated", func(t *testing.T) {
		old := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		warnIfNonDefaultRoot(installPaths{unitDir: t.TempDir(), configDir: "/custom/nenya"}, true)
		_ = w.Close()
		os.Stderr = old
		out, _ := io.ReadAll(r)
		if !strings.Contains(string(out), "Note: generated") {
			t.Errorf("stderr = %q, want a generated note", out)
		}
	})

	t.Run("warns when the unit came from the archive", func(t *testing.T) {
		old := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		warnIfNonDefaultRoot(installPaths{unitDir: t.TempDir(), configDir: "/custom/nenya", secretsFile: "/custom/nenya/secrets.json"}, false)
		_ = w.Close()
		os.Stderr = old
		out, _ := io.ReadAll(r)
		if !strings.Contains(string(out), "Warning:") || !strings.Contains(string(out), "--config-dir /custom/nenya") {
			t.Errorf("stderr = %q, want a warning with the regen command", out)
		}
	})

	t.Run("silent for the default root", func(t *testing.T) {
		old := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w
		warnIfNonDefaultRoot(installPaths{unitDir: t.TempDir(), configDir: defaultUnitConfigDir}, false)
		_ = w.Close()
		os.Stderr = old
		out, _ := io.ReadAll(r)
		if len(out) != 0 {
			t.Errorf("stderr = %q, want no output for the default root", out)
		}
	})
}

func TestUnitSpecsForPlatforms(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		specs := unitSpecsFor(goos)
		if len(specs) == 0 {
			t.Fatalf("%s: no specs", goos)
		}
		for _, s := range specs {
			if s.member == "" || s.destination == "" {
				t.Errorf("%s: incomplete spec %+v", goos, s)
			}
			if s.generated && s.init == "" {
				t.Errorf("%s: generated spec %+v has no init", goos, s)
			}
		}
		if goos == "darwin" && specs[0].destination != "com.gumieri.nenya.plist" {
			t.Errorf("darwin destination = %q", specs[0].destination)
		}
	}
}

func TestServiceUnitArgv(t *testing.T) {
	builder := func(outputs map[string]string) scriptRunner { return scriptRunner{outputs: outputs} }
	argv := "/bin/nenya service-unit --init systemd --exec-path /bin/nenya --config-dir /etc/nenya --secrets-file /etc/nenya/secrets.json"
	if _, ok := serviceUnitContent(context.Background(), builder(map[string]string{argv: "[Unit]"}), "/bin/nenya", "systemd", "/etc/nenya", "/etc/nenya/secrets.json"); !ok {
		t.Errorf("service-unit argv mismatch; want exactly %q (CONTRACT.md §4.5)", argv)
	}
}

func TestServiceUnitContentRejectsNonUnit(t *testing.T) {
	// A binary that ignores service-unit and prints something else (exit 0)
	// must not have that text written as a unit.
	key := "/bin/nenya service-unit --init systemd --exec-path /bin/nenya --config-dir /etc/nenya --secrets-file /etc/nenya/secrets.json"
	r := scriptRunner{outputs: map[string]string{key: "usage: nenya service-unit\n"}}
	if _, ok := serviceUnitContent(context.Background(), r, "/bin/nenya", "systemd", "/etc/nenya", "/etc/nenya/secrets.json"); ok {
		t.Fatal("expected ok=false for output that is not a unit")
	}
}

func TestServiceUnitContentPrefersNenyaServiceUnit(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("unit names differ on darwin")
	}

	key := "/bin/nenya service-unit --init systemd --exec-path /bin/nenya --config-dir /custom/nenya --secrets-file /custom/nenya/secrets.json"
	r := scriptRunner{outputs: map[string]string{key: "[Unit]\nDescription=nenya\nExecStart=/bin/nenya --config-dir /custom/nenya\n"}}

	content, ok := serviceUnitContent(context.Background(), r, "/bin/nenya", "systemd", "/custom/nenya", "/custom/nenya/secrets.json")
	if !ok {
		t.Fatal("expected service-unit output")
	}
	if !strings.Contains(string(content), "--config-dir /custom/nenya") {
		t.Errorf("generated unit does not reference the config root: %s", content)
	}
}

func TestServiceUnitContentFallsBackWhenAbsent(t *testing.T) {
	// scriptRunner returns an error for any unknown command, so this exercises
	// the feature-detection failure path.
	if _, ok := serviceUnitContent(context.Background(), scriptRunner{}, "/bin/nenya", "systemd", "/etc/nenya", "/etc/nenya/secrets.json"); ok {
		t.Fatal("expected ok=false when service-unit is unavailable")
	}
}

func TestInstallServiceUnitsGeneratesForCustomRoot(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("unit names differ on darwin")
	}

	extractDir := t.TempDir()
	// The socket is always taken from the archive; provide only that member,
	// proving the service is generated (not copied) when service-unit works.
	if err := os.MkdirAll(filepath.Join(extractDir, "deploy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extractDir, "deploy/nenya.socket"), []byte("[Socket]\nListenStream=8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unitDir := t.TempDir()
	configDir := "/custom/nenya"
	secretsFile := "/custom/nenya/secrets.json"

	key := "/bin/nenya service-unit --init systemd --exec-path /bin/nenya --config-dir " + configDir + " --secrets-file " + secretsFile
	r := scriptRunner{outputs: map[string]string{key: "[Unit]\nExecStart=/bin/nenya --config-dir " + configDir + "\n"}}

	if err := installServiceUnitsTo(context.Background(), r, "/bin/nenya", extractDir, unitDir, configDir, secretsFile); err != nil {
		t.Fatalf("installServiceUnitsTo: %v", err)
	}

	service, err := os.ReadFile(filepath.Join(unitDir, "nenya.service"))
	if err != nil {
		t.Fatalf("generated service unit missing: %v", err)
	}
	if !strings.Contains(string(service), "--config-dir "+configDir) {
		t.Errorf("nenya.service does not reference the config root: %q", service)
	}

	socket, err := os.ReadFile(filepath.Join(unitDir, "nenya.socket"))
	if err != nil {
		t.Fatalf("socket unit missing: %v", err)
	}
	if !strings.Contains(string(socket), "[Socket]") {
		t.Errorf("nenya.socket must come from the archive, got: %q", socket)
	}
}

func TestInstallServiceUnitsFallsBackToArchive(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("unit names differ on darwin")
	}

	extractDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(extractDir, "deploy"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, spec := range unitSpecs() {
		if err := os.WriteFile(filepath.Join(extractDir, spec.member), []byte("archive:"+spec.destination), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	unitDir := t.TempDir()

	// scriptRunner with no outputs: service-unit is "absent", so the archive
	// members are copied.
	if err := installServiceUnitsTo(context.Background(), scriptRunner{}, "/bin/nenya", extractDir, unitDir, "/etc/nenya", "/etc/nenya/secrets.json"); err != nil {
		t.Fatalf("installServiceUnitsTo: %v", err)
	}

	for _, spec := range unitSpecs() {
		data, err := os.ReadFile(filepath.Join(unitDir, spec.destination))
		if err != nil {
			t.Fatalf("fallback unit %s missing: %v", spec.destination, err)
		}
		if string(data) != "archive:"+spec.destination {
			t.Errorf("%s = %q, want the archive member", spec.destination, data)
		}
	}
}
