package containers

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

// minimalConfig is the documented bootstrap shim used only until nenya ships
// `example-config` (CONTRACT.md §4.4). It is not a copy of nenya's example; the
// canonical content is consumed from the contract command once available.
const minimalConfig = `{
  "server": {
    "listen_addr": ":8080"
  }
}
`

const EnvTemplate = `# Nenya container configuration
# Uncomment and modify as needed

# Nenya image to use
NENYA_IMAGE=ghcr.io/gumieri/nenya:latest

# Port to expose (internal is always 8080)
PORT=8080

# Additional environment variables for the container
# DEBUG=
`

type SetupConfig struct {
	ListenAddr string
	Dir        string
}

// HostPortMapping converts a listen address into a compose port mapping of the
// form "[host:]published:8080". The container always listens on 8080, so
// `:8080` becomes `8080:8080` and `127.0.0.1:9090` becomes
// `127.0.0.1:9090:8080` rather than the invalid `:8080:8080`.
func HostPortMapping(listen string) (string, error) {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return DefaultPort + ":" + DefaultPort, nil
	}
	if _, err := strconv.Atoi(listen); err == nil {
		return listen + ":" + DefaultPort, nil
	}

	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("invalid listen address %q: %w", listen, err)
	}
	if port == "" {
		return "", fmt.Errorf("invalid listen address %q: missing port", listen)
	}

	host = strings.Trim(host, "[]")
	switch {
	case host == "" || host == "0.0.0.0" || host == "::":
		return port + ":" + DefaultPort, nil
	case strings.Contains(host, ":"):
		return "[" + host + "]:" + port + ":" + DefaultPort, nil
	default:
		return host + ":" + port + ":" + DefaultPort, nil
	}
}

func Setup(cfg SetupConfig) error {
	configDir := filepath.Join(cfg.Dir, "config")
	secretsDir := filepath.Join(cfg.Dir, "secrets")

	hostPort, err := HostPortMapping(cfg.ListenAddr)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(minimalConfig), 0o644); err != nil {
			return err
		}
	}

	tmpl, err := template.New("compose").Parse(ComposeYAML)
	if err != nil {
		return err
	}
	data := struct{ HostPort string }{hostPort}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return err
	}
	composePath := filepath.Join(cfg.Dir, "compose.yml")
	if err := os.WriteFile(composePath, []byte(sb.String()), 0o644); err != nil {
		return err
	}

	envPath := filepath.Join(cfg.Dir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if err := os.WriteFile(envPath, []byte(EnvTemplate), 0o644); err != nil {
			return err
		}
	}

	return nil
}
