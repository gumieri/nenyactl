package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gumieri/nenyactl/internal/config"
	"github.com/gumieri/nenyactl/internal/detect"
	"github.com/gumieri/nenyactl/internal/nenya"
	"github.com/spf13/cobra"
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

func TestParseSetFlags(t *testing.T) {
	t.Run("parses key=value pairs with TUI literal rules", func(t *testing.T) {
		changes, err := parseSetFlags([]string{
			"server.listen_addr=:9999",
			"governance.ratelimit_max_tpm=100000",
			"discovery.enabled=true",
			"some.null=null",
			"name=my gateway",
		})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{
			"server.listen_addr":           `":9999"`, // bare strings are quoted
			"governance.ratelimit_max_tpm": "100000",
			"discovery.enabled":            "true",
			"some.null":                    "null",
			"name":                         `"my gateway"`,
		}
		for k, v := range want {
			if changes[k] != v {
				t.Errorf("%s = %s, want %s", k, changes[k], v)
			}
		}
		if len(changes) != len(want) {
			t.Errorf("changes = %v, want %v", changes, want)
		}
	})

	t.Run("last --set wins for a repeated key", func(t *testing.T) {
		changes, err := parseSetFlags([]string{"a.b=1", "a.b=2"})
		if err != nil {
			t.Fatal(err)
		}
		if changes["a.b"] != "2" {
			t.Errorf("a.b = %s, want 2 (last wins)", changes["a.b"])
		}
	})

	t.Run("rejects missing = and empty keys", func(t *testing.T) {
		if _, err := parseSetFlags([]string{"noequals"}); err == nil {
			t.Error("expected an error for a missing =")
		}
		if _, err := parseSetFlags([]string{"=value"}); err == nil {
			t.Error("expected an error for an empty key")
		}
	})

	t.Run("accepts an empty value as an empty string", func(t *testing.T) {
		changes, err := parseSetFlags([]string{"a.b="})
		if err != nil {
			t.Fatal(err)
		}
		if changes["a.b"] != `""` {
			t.Errorf("a.b = %s, want an empty JSON string", changes["a.b"])
		}
	})
}

func TestRenderChanges(t *testing.T) {
	t.Run("renders sorted key = value lines", func(t *testing.T) {
		out := config.RenderChanges(map[string]string{
			"b.key": `"2"`,
			"a.key": "true",
		})
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 2 || lines[0] != "a.key = true" || lines[1] != `b.key = "2"` {
			t.Errorf("RenderChanges = %q, want sorted key = value lines", out)
		}
	})

	t.Run("empty set renders a no-changes marker", func(t *testing.T) {
		out := config.RenderChanges(nil)
		if !strings.Contains(out, "No changes") {
			t.Errorf("RenderChanges = %q, want a no-changes marker", out)
		}
	})
}

func TestParseSetFlagValidation(t *testing.T) {
	t.Run("value containing = splits at the first =", func(t *testing.T) {
		changes, err := parseSetFlags([]string{"a.b=x=y"})
		if err != nil {
			t.Fatal(err)
		}
		if changes["a.b"] != `"x=y"` {
			t.Errorf("a.b = %s, want the quoted full value", changes["a.b"])
		}
	})

	t.Run("keys are trimmed but segments with internal spaces are rejected", func(t *testing.T) {
		if _, err := parseSetFlags([]string{"a. b=1"}); err == nil {
			t.Error("expected an error for whitespace inside a key segment")
		}
	})

	t.Run("malformed dotted keys are rejected", func(t *testing.T) {
		for _, key := range []string{"a..b", ".a", "a."} {
			if _, err := parseSetFlags([]string{key + "=1"}); err == nil {
				t.Errorf("key %q should be rejected (empty dotted segment)", key)
			}
		}
	})

	t.Run("valid JSON passthrough is kept verbatim", func(t *testing.T) {
		changes, err := parseSetFlags([]string{`a.b={"x":1}`})
		if err != nil {
			t.Fatal(err)
		}
		if changes["a.b"] != `{"x":1}` {
			t.Errorf("a.b = %s, want the verbatim JSON object", changes["a.b"])
		}
	})

	t.Run("malformed JSON passthrough fails locally", func(t *testing.T) {
		if _, err := parseSetFlags([]string{`a.b={"x":1`}); err == nil {
			t.Error("expected a local error for malformed JSON (before any write)")
		}
	})

	t.Run("non-JSON numbers (Inf, leading zeros) are quoted, not passed through", func(t *testing.T) {
		changes, err := parseSetFlags([]string{"a=Inf", "b=007"})
		if err != nil {
			t.Fatal(err)
		}
		if changes["a"] != `"Inf"` || changes["b"] != `"007"` {
			t.Errorf("a=%s b=%s, want both quoted as strings (invalid JSON numbers)", changes["a"], changes["b"])
		}
	})
}

func TestRunConfigSetFlagsDiff(t *testing.T) {
	// The --print-diff branch writes nothing and needs no nenya connection.
	configEditSet = []string{"a.b=:9999"}
	configEditDiff = true
	t.Cleanup(func() { configEditSet = nil; configEditDiff = false })

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	// The diff branch must not touch cmd or client; zero-value stand-ins
	// turn an accidental use into a detectable failure instead of a nil
	// dereference on Context().
	runErr := runConfigSetFlags(&cobra.Command{}, &nenya.Client{})
	closeErr := w.Close()
	os.Stdout = stdout
	out, readErr := io.ReadAll(r)
	closeErr2 := r.Close()
	if runErr != nil {
		t.Fatalf("runConfigSetFlags: %v", runErr)
	}
	if closeErr != nil {
		t.Fatalf("close stdout pipe: %v", closeErr)
	}
	if readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	if closeErr2 != nil {
		t.Fatalf("close read pipe: %v", closeErr2)
	}
	if !strings.Contains(string(out), `a.b = ":9999"`) {
		t.Errorf("diff output = %q, want the rendered change", out)
	}
	if !strings.Contains(string(out), "Dry run") {
		t.Errorf("diff output = %q, want a dry-run marker", out)
	}
}

func TestRunConfigEditSetPathSkipsDescribe(t *testing.T) {
	// The --set path must run before Describe: a pure --print-diff dry run
	// works without a live daemon. The seams fail loudly if Describe or host
	// detection were reached.
	savedDescribe := describeForEdit
	describeForEdit = func(ctx context.Context, client *nenya.Client) (nenya.Description, error) {
		t.Error("Describe must not be reached on the --set path")
		return nenya.Description{}, os.ErrNotExist
	}
	t.Cleanup(func() { describeForEdit = savedDescribe })

	savedResolve := resolveEditDirFn
	resolveEditDirFn = func(ctx context.Context) (dirResolution, error) {
		return dirResolution{
			Kind: dirConfigRoot,
			Info: &detect.Info{Mode: detect.ModeBareMetal, ConfigFile: "/tmp/test/config.json"},
		}, nil
	}
	t.Cleanup(func() { resolveEditDirFn = savedResolve })

	configEditSet = []string{"a.b=:9999"}
	configEditDiff = true
	t.Cleanup(func() { configEditSet = nil; configEditDiff = false })

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := runConfigEdit(&cobra.Command{}, nil)
	closeErr := w.Close()
	os.Stdout = stdout
	out, readErr := io.ReadAll(r)
	closeErr2 := r.Close()
	if runErr != nil {
		t.Fatalf("runConfigEdit: %v", runErr)
	}
	if closeErr != nil {
		t.Fatalf("close write pipe: %v", closeErr)
	}
	if readErr != nil {
		t.Fatalf("read pipe: %v", readErr)
	}
	if closeErr2 != nil {
		t.Fatalf("close read pipe: %v", closeErr2)
	}
	if !strings.Contains(string(out), "Dry run") {
		t.Errorf("output = %q, want the dry-run diff", out)
	}
}
