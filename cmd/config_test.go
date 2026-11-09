package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/nenya"
)

// stubBootstrapContent replaces the example-config probe with canned content
// for the duration of the test, keeping tests hermetic (no host nenya exec).
func stubBootstrapContent(t *testing.T, content string) {
	t.Helper()
	saved := bootstrapContentProbe
	bootstrapContentProbe = func() []byte { return []byte(content) }
	t.Cleanup(func() { bootstrapContentProbe = saved })
}

func TestBootstrapConfig(t *testing.T) {
	t.Run("creates config directory and files", func(t *testing.T) {
		stubBootstrapContent(t, `{"server":{"listen_addr":":8080"}}`)
		tmp := t.TempDir()
		if err := bootstrapConfig(tmp); err != nil {
			t.Fatalf("bootstrapConfig() error = %v", err)
		}

		configPath := filepath.Join(tmp, "config.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("config.json not created")
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config.json: %v", err)
		}
		if len(data) == 0 {
			t.Error("config.json is empty")
		}
	})

	t.Run("does not overwrite existing files", func(t *testing.T) {
		tmp := t.TempDir()
		configPath := filepath.Join(tmp, "config.json")

		customContent := `{"custom": true}`
		if err := os.WriteFile(configPath, []byte(customContent), 0o644); err != nil {
			t.Fatalf("write custom config: %v", err)
		}

		if err := bootstrapConfig(tmp); err != nil {
			t.Fatalf("bootstrapConfig() error = %v", err)
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config.json: %v", err)
		}
		if string(data) != customContent {
			t.Error("config.json was unexpectedly overwritten")
		}
	})
}

func TestRunConfigInit(t *testing.T) {
	t.Run("creates config via wrapper", func(t *testing.T) {
		stubBootstrapContent(t, `{"server":{"listen_addr":":8080"}}`)
		tmp := t.TempDir()
		saved := configDir
		configDir = tmp
		defer func() { configDir = saved }()

		if err := runConfigInit(testCmd(), nil); err != nil {
			t.Fatalf("runConfigInit() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(tmp, "config.json")); os.IsNotExist(err) {
			t.Error("config.json not created by runConfigInit")
		}
	})
}

func TestSecretsDirFor(t *testing.T) {
	info := &detect.Info{Mode: detect.ModeContainer, DataDir: "/host/deploy"}

	t.Run("container deployments ignore the host-side contract answer", func(t *testing.T) {
		res := dirResolution{Kind: dirContainerRoot, Info: info}
		desc := nenya.Description{Paths: nenya.Paths{SecretsDir: "/host/.config/nenya/secrets"}}
		if got := secretsDirFor(res, desc); got != info.SecretsDir() {
			t.Errorf("secretsDirFor = %q, want the container shim %q (describe runs host-side)", got, info.SecretsDir())
		}
	})

	t.Run("bare-metal config roots prefer the contract answer", func(t *testing.T) {
		infoBM := &detect.Info{Mode: detect.ModeBareMetal, ConfigFile: "/root/.config/nenya/config.json"}
		res := dirResolution{Kind: dirConfigRoot, Info: infoBM}
		desc := nenya.Description{Paths: nenya.Paths{SecretsDir: "/custom/nenya-secrets"}}
		if got := secretsDirFor(res, desc); got != "/custom/nenya-secrets" {
			t.Errorf("secretsDirFor = %q, want the contract answer /custom/nenya-secrets", got)
		}
	})

	t.Run("empty contract answer falls back to the local layout", func(t *testing.T) {
		infoBM := &detect.Info{Mode: detect.ModeBareMetal, ConfigFile: "/root/.config/nenya/config.json"}
		res := dirResolution{Kind: dirConfigRoot, Info: infoBM}
		if got := secretsDirFor(res, nenya.Description{}); got != infoBM.SecretsDir() {
			t.Errorf("secretsDirFor = %q, want the local shim %q", got, infoBM.SecretsDir())
		}
	})
}
