# Verifying nenyactl releases

Every nenyactl release ships a `checksums.txt` with the SHA-256 of every
artifact, and that checksum file is signed keylessly with
[cosign](https://github.com/sigstore/cosign) (Sigstore/Fulcio
certificate + Rekor transparency bundle:
`checksums.txt.sigstore.json`).

Verify **before extracting or installing anything**: first the
signature on `checksums.txt`, then the artifact hash against it.

## 1. Verify the signature on checksums.txt

Requires [cosign](https://docs.sigstore.dev/cosign/system_config/install/)
on your `PATH`.

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp \
    '^https://github\.com/gumieri/nenyactl/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

`Verified OK` means the checksum file was signed by the repository's
release workflow (`release.yml`) for a `vX.Y.Z` tag, with the OIDC
identity issued by GitHub Actions. The signature is anchored to the
workflow path and the release tag — a build from any other workflow,
ref, or repository fails this check.

## 2. Verify the artifact hash

Download the artifact(s) you want plus `checksums.txt`, then:

```bash
sha256sum -c --ignore-missing checksums.txt
```

`OK` for each downloaded artifact. Only artifacts you actually
downloaded are checked (`--ignore-missing` skips the rest).

## 3. Install

Only after both checks pass: extract the archive (or install the
`.deb`/`.rpm`/`arch` package / Homebrew formula) and continue with the
README installation section.

## Notes

- `checksums.txt.sigstore.json` is a Sigstore **bundle** (certificate +
  Rekor entry), so verification works offline-verification-style with
  no explicit Rekor lookup and no stored signing keys.
- The same verification logic runs in this repository's own nightly
  seam E2E (`.github/workflows/e2e.yml`), which downloads the latest
  release and refuses to proceed on a failed check — the commands above
  are exercised automatically on every run.
