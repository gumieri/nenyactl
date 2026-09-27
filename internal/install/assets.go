package install

// Embedded example configs were removed deliberately.
//
// Per CONTRACT.md §4.4 and the repo boundary rule, consumers must not embed a
// copy of Nenya's example config. The canonical content comes from
// `nenya example-config`. The single documented shim is BootstrapConfigContent
// in bootstrap.go (minimalConfig); internal/containers/setup.go keeps a second
// minimal shim for `containers setup` because that package cannot assume a
// nenya binary is on PATH — it should be collapsed into BootstrapConfigContent
// once `example-config` ships. Delete both then.
