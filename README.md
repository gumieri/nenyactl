# nenyactl

Command-line tool to install and manage Nenya AI Gateway using containers (Podman/Docker).

## Features

- Container-based deployment for all platforms (Linux, macOS, Windows)
- Interactive TUI for setting provider API keys
- Automatic configuration and secrets generation
- Supports both Podman and Docker (auto-detected)
- Cross-platform path resolution (XDG on Linux, Library folders on macOS, AppData on Windows)

## Installation

### Homebrew (macOS / Linux)

```bash
brew install gumieri/tap/nenyactl
```

### mise (all platforms)

```bash
# Install globally (no repo clone needed)
mise use -g go:github.com/gumieri/nenyactl/cmd/nenyactl
```

### Arch Linux (AUR)

```bash
yay -S nenyactl-bin
```

### Nix / NixOS

```bash
nix-env -iA gumieri.nenyactl
```

### Binary tarball (Linux / macOS)

```bash
# latest, Linux amd64
curl -fsSL https://github.com/gumieri/nenyactl/releases/latest/download/nenyactl_linux_amd64.tar.gz | tar -xz
sudo install -m 755 nenyactl /usr/local/bin/

# latest, macOS (Apple Silicon)
curl -fsSL https://github.com/gumieri/nenyactl/releases/latest/download/nenyactl_darwin_arm64.tar.gz | tar -xz
sudo install -m 755 nenyactl /usr/local/bin/
```

### System packages (Linux)

```bash
# Debian / Ubuntu
curl -fsSL https://github.com/gumieri/nenyactl/releases/latest/download/nenyactl_linux_amd64.deb -o nenyactl.deb
sudo dpkg -i nenyactl.deb

# Fedora / RHEL
sudo dnf install https://github.com/gumieri/nenyactl/releases/latest/download/nenyactl_linux_amd64.rpm
```

### From Source

```bash
go build -o nenyactl ./cmd/nenyactl/
install -m 755 nenyactl /usr/local/bin/
```

Or use the local mise tasks (requires cloning the repo):

```bash
mise install
```

## Quick Start

```bash
# Install Nenya binary and configure as a service
sudo nenyactl install

# Enable and start the service
nenyactl service start

# Or use containers instead (all platforms, or Windows-only)
nenyactl containers setup --start

# Check status
nenyactl service status
```

The `install` command:
- Downloads the latest Nenya binary from GitHub releases
- Installs it to `/usr/local/bin/nenya`
- **Linux**: Creates systemd units (`nenya.service` + `nenya.socket`) in `/etc/systemd/system/`
- **macOS**: Creates a launchd plist in `/Library/LaunchDaemons/com.gumieri.nenya.plist`
- **Windows**: Not supported — use `nenyactl containers setup` instead

## Usage

### Service Management (Linux / macOS)

```bash
# Start the service
nenyactl service start

# Stop the service
nenyactl service stop

# Check status
nenyactl service status

# Reload configuration (SIGHUP)
nenyactl service reload
```

### Binary Installation

```bash
# Install latest version with service configuration (requires sudo)
sudo nenyactl install

# Install specific version
sudo nenyactl install v0.1.0

# Install binary only, skip service setup
sudo nenyactl install --skip-service

# Install to user bin dir (no service setup)
nenyactl install --user --skip-service
```

### Container Management (all platforms, opt-in on Linux/macOS)

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
| `install [version]` | Install Nenya binary and service (systemd/launchd) |
| `install --skip-service` | Install binary only, no service |
| `service start/stop/status/reload` | Manage the Nenya service |
| `agents` | Configure agents via interactive TUI |
| `containers setup` | Create a new container deployment with TUI for API keys |
| `containers start` | Start Nenya containers |
| `containers stop` | Stop Nenya containers |
| `containers status` | Show container status and health check |
| `config init` | Create initial configuration |
| `secret bootstrap` | Create secrets.json with generated tokens |
| `secret generate` | Generate client tokens or API keys |
| `version` | Show version information |

## Configuration

Nenya reads configuration from `/etc/nenya/` (directory mode) or a single JSON file.

Default paths:
- **Config**: `/etc/nenya/config.json`
- **Secrets**: `/etc/nenya/secrets.json`
- **Linux service**: systemd — `nenya.service` + `nenya.socket`
- **macOS service**: launchd — `com.gumieri.nenya.plist` in `/Library/LaunchDaemons/`

Container data location (if using containers):
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

With mise (requires cloning the repo):

```bash
# Build
mise run build

# Test
mise run test

# Lint
mise run lint

# Install locally
mise run install
```

Or directly with Go:

```bash
# Build
go build -o bin/nenyactl ./cmd/nenyactl/

# Test
go test ./...

# Lint
golangci-lint run ./...

# Install
install -m 755 bin/nenyactl /usr/local/bin/
```
