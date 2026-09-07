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

The current version requires `-f` for normal operation. Providing a folder also enables downloads. Discovered URLs are printed after the download phase.

## Options

| Option | Default | Behavior |
| --- | --- | --- |
| `-u`, `--url` | None | Page URL. If omitted, read URLs from standard input. |
| `-f`, `--folder` | None | Output folder; also enables downloads. |
| `-s`, `--save-inline` | `false` | Save inline script blocks. Requires `-f`. |
| `-w`, `--workers` | `25` | Worker count, from 1 to 150. Invalid values fall back to 25. |
| `--timeout` | `5` | HTTP request timeout in seconds. |
| `--follow-redirect` | `false` | Follow HTTP redirects. |
| `-p`, `--proxy` | None | HTTP proxy URL. |
| `-H`, `--header` | None | Custom HTTP header in `Name: value` format. Repeat for multiple headers. |
| `-v`, `--verbose` | `false` | Print additional diagnostics. |
| `-d`, `--download` | `false` | Enable downloads. Still requires `-f`, which already enables them. |
| `-o`, `--output` | None | Currently opens a file but does not write results to it. |
| `--use-pb` | `false` | Accepted, but the progress bar is not implemented. |
| `-h`, `--help` | | Show command help. |

## Output and current limitations

- Downloaded files are stored as `<folder>/<host>/<filename>`. Files with the same host and filename can overwrite each other, even when their URLs differ.
- Inline scripts use random names such as `inline_<id>.txt` inside the page's host folder.
- A built-in list excludes common JavaScript library filenames and some minified variants. The list is currently not configurable.
- Extraction uses regular expressions and a fixed JavaScript filename filter. Relative URL handling and domain filtering have known correctness limitations.
- Results are not deduplicated. Diagnostic messages share standard output with URLs.
- HTTP error responses are not rejected based on status, and TLS certificate verification is currently disabled.
- Browser-generated script references, configurable file types, and recursive crawling are not supported.

## Development

```sh
go test ./...
go vet ./...
```

There are currently no automated test files. The commands above check package compilation and static diagnostics; they do not establish behavioral correctness.

## Contributing

Bug reports and focused pull requests are welcome. Use the [issue tracker](https://github.com/bp0lr/linkz/issues) and include the command, Go version, and expected behavior. For extraction bugs, a small HTML fixture is helpful.
