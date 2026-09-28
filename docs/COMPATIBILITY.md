# Nenya contract compatibility

nenyactl talks to nenya through the versioned consumer contract defined in
[nenya's `CONTRACT.md`](https://github.com/gumieri/nenya/blob/main/CONTRACT.md).
Nenya owns the mechanism; nenyactl owns the invocation. The seam is the CLI/JSON
contract, never nenya's Go packages.

## The `contract_version` we support

`nenyactl` declares its supported range in `internal/contract`, and every
install path enforces it. Immediately after writing the binary, nenyactl probes
the installed `nenya describe --json` and falls back to `version --json` to read
`contract_version`, then calls `contract.Check(v)`. The runtime contract client (`internal/nenya`) checks
`contract_version` on every `describe` as well. An out-of-range installation
fails fast with a message naming the installed and supported versions, instead
of misbehaving silently:

```text
installed nenya /usr/bin/nenya: nenya contract_version 2 is not supported by this nenyactl build (supports 1..1); update nenyactl or install a compatible nenya
```

A binary that exposes neither surface (pre-contract releases) passes the check.

## Compatibility matrix

| nenyactl | Supported nenya `contract_version` | Notes |
|----------|------------------------------------|-------|
| unreleased (pre-1.0) | 1 | Initial contract. Archive layout, `/healthz`, secrets source order, `version --json`, `paths --json`, `describe --json`, `example-config`, `service-unit`, and `config set`/`secret set`/`secret get` are **stable as of nenya v0.16.0** and consumed as the primary path. Earlier releases lack the command surfaces; those are feature-detected (see below). |

This matrix grows one row per nenyactl release. Because contract additions
(new fields, new commands) do not bump `contract_version`, a nenyactl build can
safely consume a newer additive nenya; only a breaking bump (removed/renamed
field, changed meaning/type) requires a new nenyactl release with an updated
`Supported` range.

## Feature detection

These surfaces ship in nenya **v0.16.0**, so on any current release nenyactl
consumes them directly. Feature detection remains for older installed binaries:
nenyactl runs the command and checks the exit status, and falls back to the
documented shim only when it is absent. The fallbacks are **back-compat, not the
behavior** — do not read this table as "nenyactl does X".

| Command | Shipped in | Fallback when the command is absent (older nenya) |
|---------|-----------|---------------------------------------------------|
| `version --json` | v0.16.0 | — (a binary with neither this nor `describe` passes the contract check) |
| `paths --json` | v0.16.0 | platform defaults (`/etc/nenya`, `~/.local/share/nenyactl/nenya`), with a stderr note for the default root |
| `describe --json` | v0.16.0 | contract check falls back to `version --json`; `config edit` and `agents` require `describe` and fail with an actionable error otherwise |
| `example-config` | v0.16.0 | a minimal, documented bootstrap config |
| `service-unit` | v0.16.0 | the units shipped in the release archive (`deploy/`). When present, the service unit (`nenya.service` on Linux, `com.gumieri.nenya.plist` on macOS) is generated; the systemd socket is always from the archive |
| `config set` / `secret set` | v0.16.0 | `install`'s fresh-install bootstrap hand-writes the config-root `secrets.json` (feature-detected; on v0.16.0 the write is delegated to `nenya secret set` and its reported path is verified) |
| `secret get` | v0.16.0 | the file shim reads the name-ordered `*.json` merge over the deployment's secrets directory (it deliberately ignores a bare `secrets` file, which is a `$CREDENTIALS_DIRECTORY` name only the server resolves) |

`paths --json` now reports `secrets_file` (additive in v0.16.0); nenyactl parses
it as an optional field, so it is `nil` against an older binary. `describe`
tolerates a missing config (exit 0 with a `config_not_found` diagnostic) as of
v0.16.0, which is what lets `status`/`doctor` run before a config exists.

`agents` requires `describe --json`. `client add` and `up` resolve the port from
`describe`'s effective config (so `config.d` overlays are honored), falling back
to the deployment's config file and `config.d` drop-ins when `describe` is
unavailable.

Fallbacks are marked in code and removed as older binaries age out.

## Upgrading

- **nenyactl → newer**: safe across additive contract changes.
- **nenya → newer minor**: safe; nenyactl feature-detects anything new. v0.16.0
  is the first release shipping the full consumer contract surface, and nenyactl
  consumes it directly.
- **nenya → older than v0.16.0**: still supported — the missing commands fall
  back to the documented shims above.
- **nenya → breaking contract bump**: nenyactl will fail fast with the message
  above on install, and `describe` on every command. Upgrade nenyactl (or pin
  nenya) to restore compatibility.
