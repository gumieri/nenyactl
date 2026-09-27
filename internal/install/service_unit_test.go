package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
	// No deploy/ members: the archive fallback must not be needed when
	// service-unit succeeds, proving generation (not copying) is used.
	unitDir := t.TempDir()
	configDir := "/custom/nenya"
	secretsFile := "/custom/nenya/secrets.json"

	// Both systemd units share one --init, so one key covers them.
	key := "/bin/nenya service-unit --init systemd --exec-path /bin/nenya --config-dir " + configDir + " --secrets-file " + secretsFile
	r := scriptRunner{outputs: map[string]string{key: "[Unit]\nExecStart=/bin/nenya --config-dir " + configDir + "\n"}}

	if err := installServiceUnitsTo(context.Background(), r, "/bin/nenya", extractDir, unitDir, configDir, secretsFile); err != nil {
		t.Fatalf("installServiceUnitsTo: %v", err)
	}

	for _, spec := range unitSpecs() {
		data, err := os.ReadFile(filepath.Join(unitDir, spec.destination))
		if err != nil {
			t.Fatalf("generated unit %s missing: %v", spec.destination, err)
		}
		if !strings.Contains(string(data), "--config-dir "+configDir) {
			t.Errorf("%s does not reference the config root: %q", spec.destination, data)
		}
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
