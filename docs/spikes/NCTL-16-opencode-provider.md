# NCTL-16 — OpenCode provider registration

Status: **decided** (2026-09-27). Outcome: ship configuration generation, not a
plugin or npm package.

## Question

Should nenyactl distribute the Nenya gateway to OpenCode through an npm
package/plugin (e.g. `@gumieri/opencode-nenya`) that registers the provider
automatically, instead of generating configuration?

## Findings

OpenCode V2 registers a provider through configuration, not a plugin. A custom
provider is a `providers.<id>` entry with `package`, `settings`, and `models`
([providers guide](https://opencode.ai/v2/docs/providers)):

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "providers": {
    "nenya": {
      "name": "Nenya Gateway",
      "package": "@opencode/ai/providers/openai-compatible",
      "settings": { "baseURL": "http://localhost:8080/v1", "apiKey": "nk-…" },
      "models": { "build": { "name": "build" } }
    }
  }
}
```

A plugin is for hooks/tools/transforms, not for registering a provider. The
`plugins` config field loads behavior, and the plugin guide does not offer a
provider-registration hook. So an `@gumieri/opencode-nenya` package would at
best ship the same JSON a user could paste — adding an npm publish, a version
matrix against OpenCode, and a supply-chain surface for no functional gain.

## Decision

- nenyactl generates the V2 provider block (`nenyactl client add opencode`).
  Output modes: default prints a redacted snippet; `--show-token` prints it with
  the real token; `-o/--output` writes it to a 0600 file; `--write` merges it
  into `~/.config/opencode/opencode.json`, preserving unrelated keys and
  comments. `up`/`install --connect` print the client connection info with a
  pointer to `client add --show-token`.
- No npm scope, no plugin package. Revisit only if OpenCode adds a
  provider-registration hook that a plugin could use.
- The generated shape is pinned by `TestRenderOpenCodeUsesV2Shape` so a V1
  regression fails loudly, and a legacy V1 `provider.nenya` is removed on
  merge (`TestMergeOpenCodeMigratesLegacyV1`).

## Verification

The spike is satisfied by inspection of the V2 provider/plugin documentation
plus the generated shape. A live registration check requires a running OpenCode
instance; `TestRenderOpenCodeUsesV2Shape` covers the configuration contract.
