# Linkz modernization plan

Four separate commits on one working branch. No pull requests are part of this delivery.

| Commit | Scope | Validation |
| --- | --- | --- |
| 1. Build baseline | Go 1.26.8, module cleanup, documentation and work plan. | Compilation and static analysis with the required Go version. |
| 2. Reliable collection | Listing without downloads, working `-o`, URL resolution, origin scope, TLS, HTTP errors, size limits, cancellation, and distinct storage paths. | Local regression tests for CLI, extraction, HTTP, and files. |
| 3. Less repeated work | Shared HTTP connections, page/resource deduplication, streamed downloads, precalculated library exclusions. | Request counts, connection reuse, failed-write preservation, and recorded microbenchmarks. |
| 4. Useful inventory | `--include-libs`, `--stats`, `--version`, JSONL manifest, SHA-256, offline HTML input, README and CI. | Provenance, hashes, output conflicts, writer errors, and absence of HTTP requests in local mode. |

## Decisions

- Preserve useful short flags. `-f` enables downloads in HTTP mode; `--use-pb` becomes a deprecated statistics alias.
- Keep stdout for unique resource URLs and stderr for diagnostics.
- Replace URL output and manifest files on each run. Preserve per-page provenance in the manifest even when a download is shared.
- Restrict resources and redirects to the original page origin; verify TLS certificates.
- Use stable, filesystem-safe identities for stored scripts and separate content hashes in the manifest.
- Make local HTML mode work without any HTTP requests.

## Completed

- [x] Build baseline.
- [x] Reliable collection and regression tests.
- [x] Reduced repeated work and local measurements.
- [x] Inventory features, offline mode, documentation, and CI configuration.

## Deferred

Browser automation, recursive crawling, configurable extra origins, content-based deduplication across runs, persistent cache, and automatic change monitoring. The current scope is a small JavaScript collector with a useful local inventory.

## Checks

Use local HTTP servers and HTML fixtures, not public websites. Run `go test ./...`, `go vet ./...`, and `go mod tidy -diff`. The CI configuration covers Windows/Linux and runs the race detector on Linux; configuring CI does not imply that remote jobs have run.
