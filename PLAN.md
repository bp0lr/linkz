# Linkz modernization plan

Four focused PRs, reviewed and merged in order. Each PR is based on the previous branch so its diff only contains its own changes.

| PR | Scope | Acceptance criteria |
| --- | --- | --- |
| 1. Build baseline | Require Go 1.26.8, tidy modules, document the existing CLI and this plan. | Packages compile and `go vet` passes with the required Go version. |
| 2. Reliable collection | Fix listing and `-o`, URL resolution, HTTP errors, scope, TLS defaults, cancellation, and file collisions. Add local regression tests. | Listing works without downloads; distinct URLs cannot overwrite each other; failures are visible on stderr and return a nonzero status. |
| 3. Less repeated work | Reuse HTTP connections, deduplicate page and resource requests, stream downloads to disk, and precompute library exclusions. | Local tests verify request reuse and bounded downloads; benchmarks report allocations for extraction and library filtering. |
| 4. Useful inventory | Add `--include-libs`, `--stats`, `--version`, a JSONL manifest with SHA-256, and local HTML input. Update README and add CI. | Manifest records preserve provenance and match saved bytes; local HTML mode makes no network requests; CLI examples and flags match the implementation. |

## Product scope

Linkz collects JavaScript referenced by explicitly supplied pages. Keep it small and useful for producing a local collection with provenance. Browser automation, recursive crawling, content-based deduplication across runs, and change monitoring are deferred.

## Compatibility decisions

- Keep existing short flags where they have useful behavior. `-f` continues to enable downloads.
- Make `--help` successful, validate arguments before work, and reserve stdout for results.
- Make output files replace the previous result list instead of silently accumulating stale results.
- Restrict resources and redirects to the page's origin; verify TLS certificates by default.
- Use stable, filesystem-safe filenames that distinguish complete URLs, including query strings.
- Keep proposed features out of usage documentation until implemented.

## Validation

Use local HTTP test servers and HTML fixtures. Run `go test ./...`, `go vet ./...`, and relevant benchmarks. Run the race detector where the toolchain supports it. Do not use public websites as test targets.

## Progress

- [x] PR 1: Build baseline and plan.
- [x] PR 2: Reliable collection.
- [ ] PR 3: Less repeated work.
- [ ] PR 4: Useful inventory and final documentation.
