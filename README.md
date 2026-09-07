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

## Output and inventory

Stdout contains unique external script URLs. `-o` writes the same list to a file. Logs and optional statistics go to stderr. Output order may vary with concurrent workers.

`--manifest` writes JSONL with one record per page/resource relationship, plus inline records when `-s` is used. A shared script can have multiple manifest records while being downloaded once. Page failures are recorded with `kind: "page_error"`.

| Field | Meaning |
| --- | --- |
| `schema_version` | Inventory format version, currently `1`. |
| `source` | Input page URL, local HTML filename, or `-` for HTML from stdin. |
| `page` | Final page URL used for extraction, when available. |
| `kind` | `external`, `inline`, or `page_error`. |
| `status` | `listed`, `saved`, or `error`. |
| `url` | Discovered resource URL; for inline code, the page URL. |
| `final_url`, `http_status` | Final external resource URL and successful HTTP response status, when available. |
| `file` | Saved path relative to `--folder`, using forward slashes. |
| `size_bytes`, `sha256` | Byte count and SHA-256 of saved content. Size is `0` for unsaved records; no content hash is emitted for them. |
| `inline_index` | One-based script element position for inline code. |
| `error` | Failure description, when present. |

Files are stored under `<folder>/host_<safe-host>/`:

- External scripts use `script-<identity-hash>.js`. Identity includes the full normalized URL and its query string, so different paths and query strings produce distinct names.
- Inline scripts use `inline-<identity-hash>.js`. Identity includes page URL, script position, and content hash. A changed inline block gets a new filename.
- The hash in the filename is an identity hash. The manifest's `sha256` is the hash of the actual file contents.
- Downloads stream into temporary files within the selected output root. A failed read or write leaves any previous complete file in place and removes the temporary file.
- A successful rerun replaces an external file with the same URL identity. Existing unrelated files are retained.

Both `-o` and `--manifest` replace previous contents. Input HTML, URL output, and manifest must refer to different files. These files are progressive outputs, so a failed or interrupted run can leave partial results. Completed script files remain available; a page interrupted before reporting may have saved files without manifest records.

`--stats` reports completed page results, unique external URLs, unique saved files, saved bytes, failed records, and elapsed time. Saved file totals include inline scripts. Shared downloads count once in file and byte totals; an error referenced by multiple pages counts once per failed record.

## Options

| Option | Default | Behavior |
| --- | --- | --- |
| `-u`, `--url` | None | Page URL. Otherwise read one URL per line from stdin. |
| `-f`, `--folder` | None | Storage folder; enables external downloads in HTTP mode. |
| `-o`, `--output` | None | Replace this file with the discovered URL list. |
| `-s`, `--save-inline` | `false` | Save inline JavaScript. Requires `-f`. |
| `-d`, `--download` | `false` | Explicitly request downloads. Requires `-f`, which already enables them in HTTP mode. |
| `--manifest` | None | Replace this file with a JSONL inventory. |
| `--include-libs` | `false` | Include filenames from the bundled library exclusion list. |
| `--input-html` | None | Read local HTML; `-` reads HTML from stdin. Makes no HTTP requests. |
| `--base-url` | None | Original page URL for local HTML input. Required with `--input-html`. |
| `--stats` | `false` | Print final totals to stderr. |
| `-w`, `--workers` | `25` | Maximum concurrent HTTP requests, from 1 to 150, shared across page and script workers and all hosts. |
| `--timeout` | `5` | Total HTTP request timeout in seconds, from 1 to 86400. |
| `--max-size` | `16777216` | Maximum bytes per HTTP response or local HTML input, from 1 byte to 1 GiB. |
| `--follow-redirect` | `false` | Follow redirects within the page origin, up to 10 hops. |
| `-p`, `--proxy` | None | HTTP or HTTPS proxy URL. Standard Go proxy environment variables also apply. |
| `-H`, `--header` | None | HTTP header in `Name: value` format. Repeat for multiple headers; the last value for a name wins. Overriding `Host` is not supported. |
| `-v`, `--verbose` | `false` | Print page diagnostics to stderr. |
| `--version` | | Print build version and exit successfully. |
| `-h`, `--help` | | Print help and exit successfully. |

The legacy `--use-pb` flag is a deprecated alias for `--stats`; there is no animated progress bar.

## Scope and limitations

- Resources and redirects must retain the page origin: scheme, hostname, and effective port. CDN hosts, sibling subdomains, and HTTP-to-HTTPS redirects are outside this scope. Supply the final HTTPS page URL directly when appropriate.
- TLS certificates are verified. Non-2xx responses, oversized responses, and file errors are reported. Failed external downloads remain in the URL list and have error records in the manifest.
- HTML extraction supports script elements, modules, and the first `<base href>`. JSON data blocks are not saved as inline JavaScript.
- Quoted `.js` and `.mjs` references are also collected as a best-effort fallback. This is not a JavaScript parser and can include unused references.
- The bundled library filter matches filenames and selected minified variants, case-insensitively. It does not detect library versions or inspect their contents.
- URLs are deduplicated per run, including fragment-only differences. Download successes and failures are cached for the run. There are no automatic retries or persistent cache.
- HTTP connections are reused, downloads stream to disk through a bounded worker queue, and exclusions are precalculated. A single page can download multiple scripts concurrently. HTML buffers are bounded by `--max-size` per page worker; bookkeeping grows with unique URLs.
- Linkz does not recursively crawl pages, execute JavaScript, follow module imports, deduplicate by content, or compare successive inventories automatically.

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
