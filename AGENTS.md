# AGENTS.md — nenyactl

Guidance for agent sessions rooted in this repository. This file is normative
for how work is done here. The consumer contract it implements lives in
[nenya's `CONTRACT.md`](https://github.com/gumieri/nenya/blob/main/CONTRACT.md)
and is read-only: nenyactl never edits it.

## 1. What this project is

`nenyactl` is the **deliberate parallel UX/lifecycle layer** for the Nenya AI
Gateway. It installs the `nenya` binary, bootstraps config and secrets, manages
the systemd/launchd service, scaffolds container deployments, and edits
configuration through a TUI.

Nenya itself is **zero-dependency** (Go standard library only) and must stay
that way. That guarantee is about *nenya's* `go.mod` and gateway runtime — it is
**not** a constraint on this repository.

## 2. Dependency policy (the opposite of nenya's)

`nenyactl` is expected to use third-party dependencies for CLI and TUI work.
The current direct set is intentional:

| Dependency | Why |
|------------|-----|
| `github.com/spf13/cobra` | Command tree, flags, help. |
| `github.com/charmbracelet/bubbletea` | Interactive TUIs. |
| `github.com/charmbracelet/bubbles` | Reusable TUI components. |
| `github.com/charmbracelet/lipgloss` | TUI styling. |
| `github.com/tailscale/hujson` | Comment-preserving JSONC for config editing. |
| `os/exec` (stdlib) | Service managers and container runtimes. |

Rules:

1. **Justify** every new dependency in the commit or PR body: what it replaces,
   why stdlib is insufficient, and its maintenance/licence posture.
2. **Pin** it exactly (Go modules already pin via `go.sum`); do not use `replace`
   or `+incompatible` without a comment explaining why.
3. **Keep `go.mod` truthful.** Direct imports must be in the first `require`
   block, never marked `// indirect`. `go mod tidy` must be a no-op in CI — a
   dirty `go.mod`/`go.sum` fails the build.
4. Prefer a dependency over hand-rolled protocol/parsing code that already has a
   well-maintained library; prefer stdlib when it is genuinely sufficient.

## 3. The boundary rule (non-negotiable)

> **Nenya owns the mechanism. nenyactl owns the invocation.**

Nenya is the single source of truth for:

- the config schema and the merged effective config,
- merge precedence and directory/file mode selection,
- resolved paths (config dir/file, `config.d`, secrets dir, socket),
- the secrets layout and source priority order,
- release artifact and archive member layout,
- service-unit contents,
- `/healthz` and the HTTP contract.

Nenya exposes these through the versioned contract surface
(`nenya version --json`, `describe --json`, `paths --json`, `example-config`,
`service-unit`, `config set`, `secret set`).

### 3.1 What nenyactl MUST NOT do

- **Never re-implement** nenya's loader, merge semantics, precedence, or path
  resolution. If nenyactl needs "what is in effect", it calls
  `nenya describe --json`; it does not read and merge files itself.
- **Never hardcode** `/etc/nenya`, `config.d`, `secrets.json`, or archive
  member names. Paths come from `nenya paths --json`; unit contents come from
  `nenya service-unit`; the example config comes from `nenya example-config`.
- **Never embed a copied example config.** Remove copies; consume
  `nenya example-config` instead. Any bootstrap fallback must be a documented,
  minimal compatibility shim that is deleted once the contract command ships.
- **Never import nenya Go packages.** `github.com/gumieri/nenya/...` is
  `internal`/not contract. The seam is the CLI/JSON contract, not Go.
- **Never parse service units or config out of a release archive.** Extract only
  the `nenya` member by exact name; use `nenya service-unit` for units.

### 3.2 Feature-detect the target surface

Several contract commands are still `target` (in progress on the nenya side,
NENYA-86..97). Consume a command only when it exists — attempt it and check the
exit status, then fall back to a documented shim and emit a clear message. Never
assume a `target` command exists. Nenya's `contract_version` is surfaced by
`version --json` / `describe --json`; declare the supported range and fail fast
with an actionable message outside it (`SupportedContract = [1, 1]`).

## 4. Contract facts nenyactl must honor

These are copied verbatim in intent from `CONTRACT.md` and must not drift:

- **Release archives**
  - `nenya_<version>_linux_{amd64,arm64}.tar.gz`
  - `nenya_<version>_darwin_{amd64,arm64}.tar.gz`
  - Members: `nenya` (all); `deploy/nenya.service` + `deploy/nenya.socket`
    (linux); `deploy/nenya.plist` (darwin).
  - **No config example is shipped in archives** — use `nenya example-config`.
  - Consumers **must** extract the `nenya` member by exact name.
  - Integrity: `checksums.txt` + `checksums.txt.sigstore.json` (cosign
    `sign-blob` bundle) and `*.spdx.json`. Verify the bundle against the
    expected release identity, then the archive SHA-256 against `checksums.txt`,
    **before** extracting or installing.
  - Package names: `nenya_<version>_linux_<arch>.deb`,
    `nenya_<version>_linux_<arch>.rpm`,
    `nenya_<version>_linux_<arch>.pkg.tar.zst`; AUR `nenya-bin`; Nix
    `gumieri/nur-packages` → `nenya`.
- **Health**: poll **`/healthz`** (never `/health`); unauthenticated. `/v1/*`
  and `/proxy/*` require `Authorization: Bearer <client_token|api_key_token>`.
- **Secrets source order** (first match wins):
  1. `$CREDENTIALS_DIRECTORY/secrets` (single file)
  2. `$CREDENTIALS_DIRECTORY/secrets.d/*.json`
  3. `$NENYA_SECRETS_DIR/*.json`
  4. `/run/secrets/nenya/*.json`
  `client_token` is required. Secrets files are mode `0600`.
- **Config layout**: `<config-root>/config.json` or `<config-root>/config.d/`.
  Directory mode wins over `config.json` when `config.d/` has at least one
  `*.json` (excluding `secrets.json`). The XOR is being revised; treat
  `nenya describe` as the authority.
- **Service units**: systemd `nenya.service` (+ `nenya.socket`); launchd
  `nenya.plist`, label `com.gumieri.nenya`. The shipped systemd unit wires
  secrets with `LoadCredential=secrets:/etc/nenya/secrets.json`. Regenerate with
  `nenya service-unit --secrets-file …` for a different root.
- **Signals**: `SIGHUP` reloads config/secrets (fail-closed); `SIGTERM`/`SIGINT`
  drain gracefully.

> **Do not "fix" the `deploy/nenya.service` lookup in the installer.** Extracting
> `deploy/nenya.service` and `deploy/nenya.socket` from the archive is correct
> and verified against a real release archive. The broken archive consumer was
> nenya's own `install.sh`, tracked in the Nenya project.

## 5. Code conventions

- **`cmd/` is thin.** Cobra wiring, flag parsing, output. Business logic lives
  in `internal/*` and is unit-testable without a terminal or root.
- **Dependency injection for side effects.** Exec goes through the `execer` /
  `cmdRunner` interfaces; HTTP goes through `HTTPDoer` (or the standard
  `*http.Client`). Tests supply fakes; production supplies `os/exec` /
  `http.DefaultClient`.
- **Receiver methods**, not free functions, when a type is involved.
- **GoDoc on every exported symbol.** Unexported helpers only when the name is
  self-evident.
- **Wrap errors with `%w`** and add context at each boundary
  (`fmt.Errorf("install: %w", err)`). No string-matching on error text.
- **`gofmt` clean.** Standard library layout, effective-Go idioms, no dead code.
- **TUI changes ship with golden/scripted tests** (`charmbracelet/x/exp/teatest`
  + golden files) — input sequences, not screenshots by hand.
- Never log or print secrets. `client_token`, provider keys, and API keys are
  written only to `0600` files or stdout at the user's explicit request.

## 6. Verification gates

Run the full line before moving an issue to `In Review`. The commit hash goes in
the worklog.

```bash
gofmt -l .                      # must be empty
go build ./...                  # compiles
go vet ./...                    # no findings
golangci-lint run ./...         # no findings
go test ./... -count=1          # all green (unit + integration)
go mod tidy && git diff --exit-code go.mod go.sum   # tidy is a no-op
```

Plus the gates specific to the change:

- **Coverage**: CI enforces a coverage floor over `./internal/... ./cmd/...`.
  Do not lower it to make a change pass; raise it when adding tests.
- **TUI golden tests**: `go test ./internal/agents/... ./internal/containers/... -run TestTUI`.
- **Real-release E2E**: the golden-path E2E consumes the **real latest nenya
  release** (download + checksum verify + install + configure + start + health +
  request), not synthetic fixtures. It runs nightly and on demand; it must fail
  loudly on any seam break.

CI mirrors these gates (`.github/workflows/test.yml`). A red gate is not
"flake" until reproduced.

## 7. Plane workflow (project NCTL)

Tracking lives in Plane project **Nenyactl** (`NCTL`, id
`082296ae-fa76-4bea-b6cb-248b76ec9d4d`), organised by **modules**.

Per issue:

1. Move the issue to **In Progress** before writing code.
2. Implement, then run the verification line above.
3. Add a **worklog** with: what changed, the verification command and result,
   and the commit hash.
4. Move to **In Review** (state `In Review`, `started` group) with the commit
   hash.
5. Run the `review-loop` skill (`code-reviewer` subagent), fix **all** findings,
   and only then move to **Done**.

Do not start modules that depend on nenya's `target` contract surface
(`describe`/`paths`/`example-config`) until those ship. Detect, don't assume.

## 8. Memory protocol

Wing `nenyactl` in MemPalace. Rooms:

- `task-status` — resumption points, what is in flight.
- `decisions` — architecture and boundary decisions (e.g. dependency policy).
- `gotchas` — verified bugs and traps, and which ones were **invalidated**.
- `environment` — tooling, versions, local setup.

On wake-up: `mempalace_status`, then `mempalace_search(wing="nenyactl")` and a
`task-status` search for the last resumption point. Before replying about
project history, search — never guess. After completing a module or making a
decision, add a drawer; use `mempalace_kg_add` for durable relational facts.
Durable human docs go to Outline; execution state stays in Plane.

## 9. Repository hygiene

- `.opencode/opencode.json` may contain local tool credentials. It is ignored
  via `.opencode/.gitignore` (`*`); verify with
  `git check-ignore .opencode/opencode.json` and never commit it. If a secret
  ever lands in history, rotate it and remove it from history.
- Never commit `*.key`, `*.pem`, `.env`, tokens, or generated `bin/` output.
- Secrets generated locally stay on the machine; do not paste them into Plane,
  Outline, MemPalace, logs, or commit messages.
