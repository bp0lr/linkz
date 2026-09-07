# Linkz

A Go command-line utility that extracts JavaScript links from supplied web pages, downloads matching files, and optionally saves inline scripts.

Linkz processes the pages you provide. It does not recursively visit linked pages or run JavaScript in a browser.

## Requirements

- Go **1.26.8 or newer** to build from source.
- Network access to retrieve Go modules during the first build and to fetch web pages when running Linkz.

Use a maintained Go release with its latest patch updates. See the [Go release history](https://go.dev/doc/devel/release) for support information.

## Installation

Install the published version:

```sh
go install github.com/bp0lr/linkz@latest
```

Make sure your Go binary directory is on `PATH`. By default, it is `$(go env GOPATH)/bin`.

To build this checkout:

```sh
go build -o linkz .
```

On Windows, use `go build -o linkz.exe .`.

## Usage

Download JavaScript files referenced by a page:

```sh
linkz -u https://example.com -f output
```

List script URLs without downloading their contents:

```sh
linkz -u https://example.com -o urls.txt
```

Also save inline script blocks:

```sh
linkz -u https://example.com -f output -s
```

Process a file containing one full URL per line, using a POSIX shell:

```sh
linkz -f output --timeout 10 < urls.txt
```

In PowerShell:

```powershell
Get-Content urls.txt | linkz -f output --timeout 10
```

Providing a folder enables downloads. Without a folder, Linkz only lists script URLs. `-o` writes the same URLs to a file, replacing previous contents. Diagnostics go to stderr. Results from concurrent pages may arrive in a different order between runs.

## Options

| Option | Default | Behavior |
| --- | --- | --- |
| `-u`, `--url` | None | Page URL. If omitted, read URLs from standard input. |
| `-f`, `--folder` | None | Output folder; also enables downloads. |
| `-s`, `--save-inline` | `false` | Save inline script blocks. Requires `-f`. |
| `-w`, `--workers` | `25` | Concurrent page workers, from 1 to 150. |
| `--timeout` | `5` | HTTP request timeout in seconds. |
| `--max-size` | `16777216` | Maximum bytes per response, up to 1 GiB. |
| `--follow-redirect` | `false` | Follow redirects within the page's origin, up to 10 hops. |
| `-p`, `--proxy` | None | HTTP proxy URL. |
| `-H`, `--header` | None | Custom HTTP header in `Name: value` format. Repeat for multiple headers. |
| `-v`, `--verbose` | `false` | Print additional diagnostics. |
| `-d`, `--download` | `false` | Enable downloads. Still requires `-f`, which already enables them. |
| `-o`, `--output` | None | Write discovered URLs to a file, replacing previous contents. |
| `--use-pb` | `false` | Legacy option; requesting it returns an argument error. |
| `-h`, `--help` | | Show command help. |

## Output and current limitations

- Downloaded files are stored as `<folder>/host_<safe-host>/script-<URL-hash>.js`. The complete URL, including query parameters, determines the name. Different paths and query strings produce distinct files.
- Inline scripts use stable `inline-<identity-hash>.js` names based on page URL and script position. JSON data blocks are excluded.
- Files are written through temporary files inside the selected output root. Successful reruns replace files with the same identity.
- A built-in list excludes common JavaScript library filenames and some minified variants. The list is currently not configurable.
- HTML extraction supports `script src`, modules, and the first `<base href>`. Quoted `.js` and `.mjs` references are also collected as a best-effort fallback, without parsing JavaScript syntax.
- Resources and redirects must keep the page's origin: scheme, hostname, and effective port. CDN hosts, sibling subdomains, and HTTP-to-HTTPS redirects are outside this scope.
- TLS certificates are verified. Non-2xx responses, responses over the size limit, and file errors are reported. Failed downloads remain in the URL list.
- Links are deduplicated within each page. Shared scripts across different pages can still be downloaded more than once.
- Browser-generated script references, configurable file types, and recursive crawling are not supported.

## Development

```sh
go test ./...
go vet ./...
```

Tests use local HTTP servers and HTML fixtures for extraction, CLI output, storage, response limits, origin policy, and cancellation. See [PLAN.md](PLAN.md) for the PR sequence.

Exit codes: `0` for success or help, `1` for collection or output failures, and `2` for invalid arguments. Ctrl+C cancels pending HTTP work. Already completed files remain available after a failure.

## Contributing

Bug reports and focused pull requests are welcome. Use the [issue tracker](https://github.com/bp0lr/linkz/issues) and include the command, Go version, and expected behavior. For extraction bugs, a small HTML fixture is helpful.
