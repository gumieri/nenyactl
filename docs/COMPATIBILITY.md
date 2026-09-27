# Nenya contract compatibility

nenyactl talks to nenya through the versioned consumer contract defined in
[nenya's `CONTRACT.md`](https://github.com/gumieri/nenya/blob/main/CONTRACT.md).
Nenya owns the mechanism; nenyactl owns the invocation. The seam is the CLI/JSON
contract, never nenya's Go packages.

## The `contract_version` we support

`nenyactl` declares its supported range in `internal/contract`, and every
install path enforces it. Immediately after writing the binary, nenyactl probes
the installed `nenya describe --json` (target) and falls back to
`version --json` (stable) to read `contract_version`, then calls
`contract.Check(v)`. The runtime contract client (`internal/nenya`) checks
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
| unreleased (pre-1.0) | 1 | Initial contract. Archive layout, `/healthz`, secrets source order, `version --json`, `example-config`, and `service-unit` are stable; `paths`/`describe`/`config set`/`secret set` are `target` and feature-detected. |

This matrix grows one row per nenyactl release. Because contract additions
(new fields, new commands) do not bump `contract_version`, a nenyactl build can
safely consume a newer additive nenya; only a breaking bump (removed/renamed
field, changed meaning/type) requires a new nenyactl release with an updated
`Supported` range.

## Feature detection

Target surfaces are consumed only when present. nenyactl runs the command and
checks the exit status; until it ships, nenyactl uses a documented fallback:

| Command | Status | Fallback until it ships |
|---------|--------|-------------------------|
| `version --json` | stable | — |
| `paths --json` | target | platform defaults (`/etc/nenya`, `~/.local/share/nenyactl/nenya`) |
| `describe --json` | target | contract check falls back to `version --json`; `config edit` requires `describe` and fails with an actionable error otherwise |
| `example-config` | stable | a minimal, documented bootstrap config |
| `service-unit` | stable | the unit shipped in the release archive (`deploy/`) — used only when `service-unit` is absent. `nenya.service` is generated; the socket is always from the archive |
| `config set` / `secret set` | target | none — nenyactl no longer edits config or secrets files directly; the command fails until the contract ships |

`agents`, `client add`, and `up` also require `describe --json`; without it they
fail with an actionable message (port resolution falls back to the deployment's
config file and `config.d` drop-ins, matching nenya's directory layout).

Fallbacks are marked in code and are removed as the contract commands ship.

## Upgrading

- **nenyactl → newer**: safe across additive contract changes.
- **nenya → newer minor**: safe; nenyactl features-detects anything new.
- **nenya → breaking contract bump**: nenyactl will fail fast with the message
  above on install (and `doctor`, once it ships). Upgrade nenyactl (or pin
  nenya) to restore compatibility.
