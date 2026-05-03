package containers

import (
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const ExampleConfig = `{
  // Server — see docs/CONFIGURATION.md for all options
  "server": {
    "listen_addr": ":8080"
  },

  // Governance: rate limiting, truncation, routing
  "governance": {
    "ratelimit_max_tpm": 250000,
    "ratelimit_max_rpm": 15
  },

  // Auto-discovery: fetches model catalogs from configured providers
  // and generates auto_reasoning, auto_vision, auto_fast, etc.
  "discovery": {
    "enabled": true,
    "auto_agents": true
  },

  // Prefix cache alignment for better upstream cache hits
  "prefix_cache": {
    "enabled": true
  },

  // Text compaction: minifies JSON, normalizes whitespace
  "compaction": {
    "enabled": true,
    "json_minify": true
  },

  // Security filter: regex-based secret redaction
  // "security_filter": {
  //   "enabled": true,
  //   "engine": { "provider": "ollama", "model": "qwen2.5-coder:7b" }
  // },

  // Agents: named model groups with fallback chains
  // You can use model names directly or define agents here.
  // The auto_discovery above creates agents like auto_fast, auto_reasoning.
  // "agents": {
  //   "build": {
  //     "strategy": "fallback",
  //     "models": ["gemini-2.5-flash", "deepseek-v4-flash"]
  //   }
  // }

  // Add custom provider URLs here (built-in ones don't need entries)
  // "providers": {
  //   "openai": { "url": "https://api.openai.com/v1/chat/completions", "auth_style": "bearer" }
  // }
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

func Setup(cfg SetupConfig) error {
	configDir := filepath.Join(cfg.Dir, "config")
	secretsDir := filepath.Join(cfg.Dir, "secrets")

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(secretsDir, 0o600); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(ExampleConfig), 0o644); err != nil {
			return err
		}
	}

	tmpl, err := template.New("compose").Parse(ComposeYAML)
	if err != nil {
		return err
	}
	data := struct{ ListenAddr string }{cfg.ListenAddr}
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
