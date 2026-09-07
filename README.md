# Linkz

Collect JavaScript from web pages and keep a local inventory with provenance and SHA-256 hashes.

Linkz processes the pages you supply. It extracts script references, optionally downloads files and inline code, and records which page each script came from. It also works with saved HTML without making HTTP requests.

## Requirements and installation

Go **1.26.8 or newer** is required to build from source. Use a maintained Go release with current patch updates; see the [Go release history](https://go.dev/doc/devel/release).

Install the published version:

```sh
go install github.com/bp0lr/linkz@latest
```

Make sure your Go binary directory is on `PATH`. By default, it is `$(go env GOPATH)/bin`. Installation with `@latest` uses the published repository version, not uncommitted changes in a local checkout.

Build this checkout:

```sh
go build -o linkz .
```

On Windows, use `go build -o linkz.exe .`.

## Quick start

List script URLs without downloading them:

```sh
linkz -u https://example.com -o urls.txt
```

Download scripts, save inline code, and write an inventory:

```sh
linkz -u https://example.com -f output -s --manifest manifest.jsonl --stats
```

Include common library files that Linkz otherwise excludes:

```sh
linkz -u https://example.com -f output --include-libs
```

Process one full page URL per line using a POSIX shell:

```sh
linkz -f output --manifest manifest.jsonl --timeout 10 < urls.txt
```

In PowerShell:

```powershell
Get-Content urls.txt | linkz -f output --manifest manifest.jsonl --timeout 10
```

## Additional script origins

Allow a specific CDN while keeping all other external origins excluded:

```sh
linkz -u https://example.com --allow-origin https://cdn.example.com -f output --manifest manifest.jsonl
```

Repeat `--allow-origin` for multiple origins. Each value must contain a scheme and host, with an optional port. Paths, credentials and wildcards are not accepted. `--follow-redirect` is still required to follow redirects. Redirects retain the original input page's scope, plus the explicit allowlist.

Custom `-H` headers are sent only to the input page's own origin. Allowed CDNs receive the default Linkz headers, including when reached through redirects. Downloads are deduplicated within each input origin; representations requested from different input origins use separate storage identities.

## Saved HTML without network requests

Use `--base-url` to identify the original page and resolve relative references:

```sh
linkz --input-html page.html --base-url https://example.com/docs/index.html -o urls.txt
```

Save inline code and its inventory from a local page:

```sh
linkz --input-html page.html --base-url https://example.com/ -f output -s --manifest manifest.jsonl
```

Use `--input-html -` to read HTML from stdin. This mode never fetches the base URL or external scripts. `-f` supplies storage for inline scripts only. `--url` and `--download` cannot be combined with local HTML input.

## Compare inventories

Compare two local snapshots without network requests:

```sh
linkz diff before.jsonl after.jsonl -o changes.jsonl
```

The command writes deterministic JSONL differences to stdout and optionally `-o`, with totals on stderr. Changes are `added`, `removed`, `changed`, `unchanged`, or `unknown`. Unchanged entries are omitted unless `--all` is supplied. Successful comparisons return exit code 0 even when differences are found; invalid input or I/O failures return 1, and argument errors return 2.

External references are matched by input page and resource URL. Inline blocks are matched by page and script position, with content changes determined by SHA-256. Local HTML snapshots use their base page URL, so different input filenames can be compared. A renamed bundle appears as an addition and removal; inline positions can shift when script elements are inserted.

Manifest version 2 includes a completion record for every processed page, including pages with no scripts. Missing or failed pages are not treated as removals. Listed-only or failed resource records cannot establish content equality and appear as `unknown`. Version 1 manifests remain readable, but their missing completion markers limit conclusions about additions, removals, and page completeness. Malformed records and conflicting duplicates are rejected.

Compare runs with matching input pages, origin allowlists, library filters, and inline settings. Differences describe the inventories, not proof of changes on the live website. Each input must be a regular JSONL file of at most 256 MiB, with records no larger than 8 MiB.

## Revalidate saved scripts

Save an initial inventory, then reuse its validators on a later run with the same storage folder:

```sh
linkz -u https://example.com -f output --manifest before.jsonl
linkz -u https://example.com -f output --cache-from before.jsonl --manifest after.jsonl --stats
linkz diff before.jsonl after.jsonl
```

`--cache-from` reads the previous manifest; no separate cache database is needed. Eligible scripts use `If-None-Match` with an ETag, or `If-Modified-Since` with Last-Modified when no ETag is available. A `304` response reuses the local file after checking its size and SHA-256. Missing or modified local files trigger a full download. A `200` response replaces the saved content and updates its metadata. Request failures remain errors; stale files are not reported as successful downloads.

HTML pages are always fetched. Revalidation requires HTTP mode, `--folder`, a direct resource response and a valid validator. Requests carrying custom `-H` headers, responses with `Cache-Control: no-store`, and `Vary` fields other than `Accept-Encoding` or `User-Agent` are excluded. Allowed CDNs can still be revalidated because they receive only default headers. Redirected responses and older records without cache metadata use full downloads. Every eligible run contacts the server; Linkz does not implement freshness-based cache serving.

Write the new inventory to a different file. Keep matching input pages and options when comparing snapshots. External files are replaced on content changes, so retain separate folders if you also need historical script bodies.

## Output and inventory

Stdout contains unique external script URLs. `-o` writes the same list to a file. Logs and optional statistics go to stderr. Output order may vary with concurrent workers.

`--manifest` writes JSONL with one record per page/resource relationship, plus inline records when `-s` is used. A shared script can have multiple manifest records while being downloaded once within its input origin. Page failures use `kind: "page_error"`; successfully processed pages end with `kind: "page"` and `status: "processed"`.

| Field | Meaning |
| --- | --- |
| `schema_version` | Inventory format version, currently `2`. |
| `source` | Input page URL, local HTML filename, or `-` for HTML from stdin. |
| `origin` | Input page origin used for request scope and header isolation. |
| `page` | Final page URL used for extraction, when available. |
| `kind` | `external`, `inline`, `page`, or `page_error`. |
| `status` | `listed`, `saved`, `error`, or `processed` for page completion records. |
| `url` | Discovered resource URL; for inline code, the page URL. |
| `final_url`, `http_status` | Final external resource URL and successful HTTP response status, when available. |
| `file` | Saved path relative to `--folder`, using forward slashes. |
| `size_bytes`, `sha256` | Byte count and SHA-256 of saved content. Size is `0` for unsaved records; no content hash is emitted for them. |
| `inline_index` | One-based script element position for inline code. |
| `etag`, `last_modified` | Valid HTTP validators supplied by the resource response, when available. |
| `cacheable` | Whether the request and response satisfy Linkz's revalidation policy. Reuse also requires a valid validator and verified local file. |
| `reused` | `true` when a `304` response allowed reuse of the verified local file. |
| `error` | Failure description, when present. |

Files are stored under `<folder>/host_<safe-host>/`:

- External scripts use `script-<identity-hash>.js`. Identity includes the full normalized URL and its query string. For cross-origin resources it also includes the input page origin, preventing different request contexts from overwriting each other.
- Inline scripts use `inline-<identity-hash>.js`. Identity includes page URL, script position, and content hash. A changed inline block gets a new filename.
- The hash in the filename is an identity hash. The manifest's `sha256` is the hash of the actual file contents.
- Downloads stream into temporary files within the selected output root. A failed read or write leaves any previous complete file in place and removes the temporary file.
- A successful rerun replaces an external file with the same URL identity. Existing unrelated files are retained.

Both `-o` and `--manifest` replace previous contents. Input HTML, URL output, manifest, and `--cache-from` must refer to different files. These files are progressive outputs, so a failed or interrupted run can leave partial results. Completed script files remain available; a page interrupted before reporting may have saved files without manifest records.

`--stats` reports completed page results, unique external URLs, unique saved files, saved bytes, failed records, reused files, downloaded bytes, and elapsed time. Saved file and byte totals include inline scripts and reused files. `downloaded_bytes` counts successfully saved external response body bytes, excluding HTML, inline scripts, reused content, and transfer overhead. Shared downloads count once in file and byte totals; an error referenced by multiple pages counts once per failed record.

## Options

| Option | Default | Behavior |
| --- | --- | --- |
| `-u`, `--url` | None | Page URL. Otherwise read one URL per line from stdin. |
| `-f`, `--folder` | None | Storage folder; enables external downloads in HTTP mode. |
| `-o`, `--output` | None | Replace this file with the discovered URL list. |
| `-s`, `--save-inline` | `false` | Save inline JavaScript. Requires `-f`. |
| `-d`, `--download` | `false` | Explicitly request downloads. Requires `-f`, which already enables them in HTTP mode. |
| `--manifest` | None | Replace this file with a JSONL inventory. |
| `--cache-from` | None | Revalidate saved scripts from this prior manifest. Requires `-f` and HTTP input. |
| `--include-libs` | `false` | Include filenames from the bundled library exclusion list. |
| `--input-html` | None | Read local HTML; `-` reads HTML from stdin. Makes no HTTP requests. |
| `--base-url` | None | Original page URL for local HTML input. Required with `--input-html`. |
| `--stats` | `false` | Print final totals to stderr. |
| `-w`, `--workers` | `25` | Maximum concurrent HTTP requests, from 1 to 150, shared across page and script workers and all hosts. |
| `--timeout` | `5` | Total HTTP request timeout in seconds, from 1 to 86400. |
| `--max-size` | `16777216` | Maximum bytes per HTTP response or local HTML input, from 1 byte to 1 GiB. |
| `--follow-redirect` | `false` | Follow redirects within allowed origins, up to 10 hops. |
| `--allow-origin` | None | Additional exact HTTP(S) origin for scripts and redirects. Repeat for multiple origins. |
| `-p`, `--proxy` | None | HTTP or HTTPS proxy URL. Standard Go proxy environment variables also apply. |
| `-H`, `--header` | None | HTTP header in `Name: value` format. Repeat for multiple headers; the last value for a name wins. Overriding `Host` is not supported. |
| `-v`, `--verbose` | `false` | Print page diagnostics to stderr. |
| `--version` | | Print build version and exit successfully. |
| `-h`, `--help` | | Print help and exit successfully. |

The legacy `--use-pb` flag is a deprecated alias for `--stats`; there is no animated progress bar.

## Scope and limitations

- Resources and redirects must match the input page origin or an explicit `--allow-origin` value, including scheme and effective port. There is no implicit trust of sibling subdomains or all CDNs.
- TLS certificates are verified. Non-2xx responses other than eligible conditional `304` responses, oversized responses, and file errors are reported. Failed external downloads remain in the URL list and have error records in the manifest.
- HTML extraction supports script elements, modules, and the first `<base href>`. JSON data blocks are not saved as inline JavaScript.
- Quoted `.js` and `.mjs` references are also collected as a best-effort fallback. This is not a JavaScript parser and can include unused references.
- The bundled library filter matches filenames and selected minified variants, case-insensitively. It does not detect library versions or inspect their contents.
- URLs are deduplicated per run, including fragment-only differences. Download successes and failures are cached for the run. Cross-run revalidation is opt-in with `--cache-from`; there are no automatic retries after request failures.
- HTTP connections are reused, downloads stream to disk through a bounded worker queue, and exclusions are precalculated. A single page can download multiple scripts concurrently. HTML buffers are bounded by `--max-size` per page worker; bookkeeping grows with unique URLs.
- Linkz does not recursively crawl pages, execute JavaScript, follow module imports, deduplicate by content, or schedule comparisons automatically.

Exit codes: `0` for success, help, or version; `1` for collection or output failures; `2` for invalid arguments. Ctrl+C cancels pending HTTP work.

## Development

```sh
go test ./...
go vet ./...
go mod tidy -diff
```

Tests use local HTTP servers and HTML fixtures. CI is configured for Windows and Linux with Go 1.26.8 and 1.27.1, including the race detector on Linux.

To reproduce the focused performance checks:

```sh
go test ./static -run '^$' -bench BenchmarkExist -benchmem
go test . -run '^$' -bench BenchmarkExtract -benchmem
```

On Windows amd64 with Go 1.26.8 and a Ryzen 9 3900X, the median lookup time for `app.js` changed from 241223 ns/op and 5000 allocations to 15.34 ns/op and zero allocations across three 200 ms runs. This measures the library filter only; map initialization is outside the measurement, and it is not an end-to-end download speed claim.

For release builds, set the version explicitly:

```sh
go build -ldflags "-X main.version=vX.Y.Z" -o linkz .
```

Otherwise, `--version` uses module or VCS build information when available and falls back to `dev`.

## Contributing

Bug reports and focused contributions are welcome through the [issue tracker](https://github.com/bp0lr/linkz/issues). Include your command, Go version, and expected behavior. For extraction bugs, a small HTML fixture is helpful.
