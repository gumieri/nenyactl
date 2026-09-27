# nenyactl

Command-line tool to install and manage the Nenya AI Gateway.

## What it does

- Install the `nenya` binary (verified: SHA-256 + cosign) and run it as a
  systemd/launchd service, bootstrapping config and secrets.
- Or scaffold a container deployment (Podman or Docker, auto-detected).
- Interactive TUI for agents, provider API keys, and config editing.
- Cross-platform path resolution (XDG on Linux, Library folders on macOS,
  AppData on Windows).

nenyactl is a separate, deliberately non-zero-dependency client. Nenya owns the
mechanism (config schema, merge precedence, paths, release layout); nenyactl
owns the invocation and experience, consuming nenya's contract.

## Installation

> **Status: pre-release.** nenyactl has no published release yet, so there are
> no tarballs and no package-manager channels. The commands below are the ones
> that will work once the first release ships; until then, build from source.

### From Source (works today)

```bash
git clone https://github.com/gumieri/nenyactl
cd nenyactl
go build -o nenyactl ./cmd/nenyactl/
install -m 755 nenyactl /usr/local/bin/   # or ~/.local/bin
```

### Binary tarball (Linux / macOS) — after first release

```bash
# Linux amd64
curl -fsSL https://github.com/gumieri/nenyactl/releases/latest/download/nenyactl_<version>_linux_amd64.tar.gz | tar -xz
sudo install -m 755 nenyactl /usr/bin/

# macOS arm64
curl -fsSL https://github.com/gumieri/nenyactl/releases/latest/download/nenyactl_<version>_darwin_arm64.tar.gz | tar -xz
sudo install -m 755 nenyactl /usr/bin/
```

### Homebrew (macOS / Linux) — after first release

```bash
brew install gumieri/tap/nenyactl
```

### Arch Linux (AUR) — after first release

```bash
yay -S nenyactl-bin
```

### Nix / NixOS — after first release

```bash
nix-env -iA gumieri.nenyactl
```

### System packages (Linux) — after first release

```bash
# Debian / Ubuntu
sudo dpkg -i nenyactl_<version>_linux_amd64.deb

# Fedora / RHEL
sudo dnf install nenyactl_<version>_linux_amd64.rpm
```

## Quick Start

```bash
# Bare metal (Linux/macOS): download, verify, install the service,
# bootstrap config + secrets (secrets.json is created mode 0600).
sudo nenyactl install

# Check status (health is /healthz)
nenyactl service status

# Or use containers instead (all platforms, including Windows)
nenyactl containers setup --start
```

`nenyactl install`:

- resolves the latest nenya release (or a specific version),
- downloads `checksums.txt` and its cosign bundle, verifies the signature and
  the archive SHA-256 **before** installing anything (use `--skip-verify` only
  for air-gapped workflows),
- installs the binary to `/usr/bin/nenya`,
- creates `<config-root>/config.json` and `<config-root>/secrets.json`
  (with a generated client token, never overwriting existing files),
- installs the shipped systemd units (`nenya.service` + `nenya.socket`) and
  runs `systemctl daemon-reload` + `enable --now nenya.socket`,
- macOS: installs the launchd plist and loads it.

Flags:

| Flag | Effect |
|------|--------|
| `--user` | Install the binary to `~/.local/bin` and bootstrap user config; no system service, no system writes. |
| `--skip-service` | Install the binary only; no config, secrets, or service. |
| `--skip-verify` | Skip cosign signature verification (SHA-256 is still enforced). Not recommended. |

## Usage

### Service Management (Linux / macOS)

```bash
nenyactl service start
nenyactl service stop
nenyactl service status
nenyactl service reload   # SIGHUP; preferred over restart for config changes
```

### Container Management (all platforms)

```bash
nenyactl containers setup            # scaffold config/, secrets/, compose.yml, .env
nenyactl containers setup --dir ./my-nenya
nenyactl containers setup --start
nenyactl containers start
nenyactl containers stop
nenyactl containers status           # reads the published port and client token from the deployment
```

The container listens on 8080 internally; `--listen 9090` publishes `9090:8080`.

### Configuration and Secrets

```bash
# Create the config file (bare metal defaults to /etc/nenya, or --dir)
nenyactl config init --dir /path/to/config

# Edit config (and agents) via TUI; auto-detects bare-metal vs container
nenyactl config edit

# Agents: auto-generated or custom
nenyactl agents
nenyactl agents --dir ~/.local/share/nenyactl/nenya   # mode auto-detected; or --mode container

# Secrets
nenyactl secret bootstrap --dir /path/to/config
nenyactl secret generate --type client
nenyactl secret generate --type apikey --name my-app
```

> **Config layout caveat.** Nenya reads `<config-root>/config.json` *or*
> `<config-root>/config.d/*.json` — creating any `config.d/*.json` makes it
> ignore `config.json` entirely. Prefer editing the single file the detected
> layout uses.

### Version

```bash
nenyactl version
```

## Commands

| Command | Description |
|---------|-------------|
| `install [version]` | Download (verified), install, bootstrap config/secrets, enable the service |
| `install --user` | User binary + user config; no system writes |
| `install --skip-service` | Binary only |
| `service start/stop/status/reload` | Manage the nenya service |
| `agents [--dir] [--mode]` | Configure agents via TUI |
| `containers setup/start/stop/status` | Container deployment |
| `config init` | Create the initial configuration |
| `config edit` | Interactive config editor |
| `secret bootstrap` | Create secret files with a generated client token |
| `secret generate` | Generate client tokens or API keys |
| `version` | Show version information |

## Configuration

Nenya reads configuration from `/etc/nenya/` (directory mode) or a single JSON
file. Default paths:

- **Config**: `/etc/nenya/config.json` (or `/etc/nenya/config.d/`)
- **Secrets**: `/etc/nenya/secrets.json` (mode 0600)
- **Linux service**: systemd — `nenya.service` + `nenya.socket`
- **macOS service**: launchd — `com.gumieri.nenya.plist` in `/Library/LaunchDaemons/`

Container data location (if using containers):

- **Linux**: `~/.local/share/nenyactl/nenya`
- **macOS**: `~/Library/Application Support/nenyactl/nenya`
- **Windows**: `%LOCALAPPDATA%\nenyactl\nenya`

See the [Nenya documentation](https://github.com/gumieri/nenya) for the full
configuration reference.

## Secrets

Container deployments store secrets in `secrets/*.json` (merged in name order);
bare metal uses a single `secrets.json`. Files are mode 0600.

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

Nenya's secrets source order (first match wins): `$CREDENTIALS_DIRECTORY/secrets`,
`$CREDENTIALS_DIRECTORY/secrets.d/*.json`, `$NENYA_SECRETS_DIR/*.json`,
`/run/secrets/nenya/*.json`.

Authenticate requests with the client token:

```bash
curl -H "Authorization: Bearer $(jq -r '.client_token' /etc/nenya/secrets.json)" \
  -d '{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"Hello!"}]}' \
  http://localhost:8080/v1/chat/completions
```

Health is `GET /healthz` (unauthenticated). `/v1/*` requires the bearer token.

## Development

```bash
# Build
go build -o bin/nenyactl ./cmd/nenyactl/

# Test (unit + integration)
go test ./... -count=1

# Full verification line
gofmt -l . && go vet ./... && golangci-lint run ./... && go mod tidy && git diff --exit-code go.mod go.sum

# Lint
golangci-lint run ./...

# Install locally
install -m 755 bin/nenyactl /usr/local/bin/
```

### Seam E2E (real nenya release)

```bash
NENYACTL_E2E=1 go test ./test/e2e/... -count=1 -timeout 15m
```

Downloads the latest nenya release, verifies its checksum, asserts the archive
layout, then drives the golden path (health → authenticated request → streamed
completion → clean shutdown). Runs nightly in CI and on demand.
