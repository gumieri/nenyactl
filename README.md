# nenyactl

Command-line tool to install and manage Nenya AI Gateway using containers (Podman/Docker).

## Features

- Container-based deployment for all platforms (Linux, macOS, Windows)
- Interactive TUI for setting provider API keys
- Automatic configuration and secrets generation
- Supports both Podman and Docker (auto-detected)
- Cross-platform path resolution (XDG on Linux, Library folders on macOS, AppData on Windows)

## Installation

### From Source

```bash
go build -o nenyactl ./cmd/nenyactl/
install -m 755 nenyactl /usr/local/bin/
```

Or with mise:

```bash
mise run build
```

## Quick Start

```bash
# Set up Nenya with interactive API key entry
nenyactl containers setup

# Start the containers
nenyactl containers start

# Check status and health
nenyactl containers status
```

The setup command:
- Creates a data directory (default: `~/.local/share/nenyactl/nenya`)
- Generates `config/config.json` with example configuration
- Generates `secrets/01-client.json` with a random authentication token
- Prompts you for provider API keys (Gemini, DeepSeek, Anthropic, OpenAI, Mistral, xAI) via TUI
- Creates `compose.yml` ready for `podman compose` or `docker compose`

## Usage

### Container Management

```bash
# Create a new deployment (interactive API key setup)
nenyactl containers setup

# Create in custom directory
nenyactl containers setup --dir ./my-nenya

# Create and auto-start
nenyactl containers setup --start

# Start containers
nenyactl containers start

# Stop containers
nenyactl containers stop

# Show status and health check
nenyactl containers status
```

### Binary Installation

For advanced use cases, you can download the nenya binary directly:

```bash
# Install latest version
sudo nenyactl install

# Install specific version
sudo nenyactl install v0.1.0

# Install to user bin dir (no sudo)
nenyactl install --user
```

### Configuration Management

```bash
# Initialize system config
sudo nenyactl config init

# Initialize in custom directory
nenyactl config init --dir /path/to/config
```

### Agent Management

```bash
# Configure agents via interactive TUI
nenyactl agents

# Switch between auto-agents (true/false)
# Edit agents: add, update, delete, reorder
# Select models per agent from provider registry
```

### Secret Management

```bash
# Bootstrap secrets with generated client token
sudo nenyactl secret bootstrap

# Generate a client token
nenyactl secret generate --type client

# Generate an API key
nenyactl secret generate --type apikey --name my-app
```

### Check Version

```bash
nenyactl version
```

## Commands

| Command | Description |
|---------|-------------|
| `agents` | Configure agents via interactive TUI |
| `containers setup` | Create a new container deployment with TUI for API keys |
| `containers start` | Start Nenya containers |
| `containers stop` | Stop Nenya containers |
| `containers status` | Show container status and health check |
| `install [version]` | Download and install nenya binary |
| `config init` | Create initial configuration |
| `secret bootstrap` | Create secrets.json with generated tokens |
| `secret generate` | Generate client tokens or API keys |
| `version` | Show version information |

## Configuration

Nenya reads configuration from `/etc/nenya/` (directory mode) or a single JSON file.

Default container data location:
- **Linux**: `~/.local/share/nenyactl/nenya`
- **macOS**: `~/Library/Application Support/nenyactl/nenya`
- **Windows**: `%LOCALAPPDATA%\nenyactl\nenya`

See [Nenya documentation](https://github.com/gumieri/nenya) for full configuration reference.

## Secrets

Secrets are stored in `secrets/01-client.json` (client token) and `secrets/02-providers.json` (provider keys) with mode 0600.

Format:

```json
{
  "client_token": "nk-...",
  "provider_keys": {
    "gemini": "AIza...",
    "deepseek": "sk-..."
  }
}
```

Use the generated client token to authenticate requests:

```bash
curl -H "Authorization: Bearer $(jq -r '.client_token' secrets/01-client.json)" \
  -d '{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"Hello!"}]}' \
  http://localhost:8080/v1/chat/completions
```

## Development

```bash
# Build
go build -o bin/nenyactl ./cmd/nenyactl/

# Test
go test ./...

# Lint
golangci-lint run ./...

# Install locally
mise run install
```
