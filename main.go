package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	web "github.com/bp0lr/linkz/fetch"
	files "github.com/bp0lr/linkz/fileutils"
	"github.com/spf13/pflag"
)

type options struct {
	url, output, folder, proxy           string
	manifest, inputHTML, baseURL         string
	headers                              []string
	allowedOrigins                       []string
	workers, timeout                     int
	maxSize                              int64
	download, inline, redirects, verbose bool
	includeLibs, stats                   bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, input io.Reader, output, diagnostics io.Writer) int {
	if len(args) > 0 && args[0] == "diff" {
		return runDiff(args[1:], output, diagnostics)
	}
	var o options
	var help, showVersion bool
	flags := pflag.NewFlagSet("linkz", pflag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.StringVarP(&o.url, "url", "u", "", "Page URL; otherwise read URLs from stdin")
	flags.StringVarP(&o.folder, "folder", "f", "", "Folder for downloaded scripts")
	flags.StringVarP(&o.output, "output", "o", "", "Write URLs to this file (replace existing contents)")
	flags.BoolVarP(&o.download, "download", "d", false, "Download scripts; requires --folder")
	flags.BoolVarP(&o.inline, "save-inline", "s", false, "Save inline JavaScript; requires --folder")
	flags.BoolVarP(&o.verbose, "verbose", "v", false, "Print page diagnostics to stderr")
	flags.IntVarP(&o.workers, "workers", "w", 25, "Maximum concurrent HTTP requests (1-150)")
	flags.IntVar(&o.timeout, "timeout", 5, "HTTP request timeout in seconds")
	flags.Int64Var(&o.maxSize, "max-size", 16<<20, "Maximum bytes per response (1-1073741824)")
	flags.BoolVar(&o.redirects, "follow-redirect", false, "Follow redirects within allowed origins")
	flags.StringArrayVar(&o.allowedOrigins, "allow-origin", nil, "Additional exact HTTP(S) origin for scripts and redirects (repeatable)")
	flags.StringVarP(&o.proxy, "proxy", "p", "", "HTTP or HTTPS proxy URL")
	flags.StringArrayVarP(&o.headers, "header", "H", nil, "HTTP header in Name: value format (repeatable)")
	flags.BoolVar(&o.includeLibs, "include-libs", false, "Include bundled library filenames")
	flags.BoolVar(&o.stats, "stats", false, "Print final collection totals to stderr")
	flags.BoolVar(&o.stats, "use-pb", false, "Alias for --stats")
	flags.MarkDeprecated("use-pb", "use --stats")
	flags.BoolVar(&showVersion, "version", false, "Show build version")
	flags.StringVar(&o.manifest, "manifest", "", "Write a JSONL inventory (replace existing contents)")
	flags.StringVar(&o.inputHTML, "input-html", "", "Read local HTML without HTTP requests; use - for stdin")
	flags.StringVar(&o.baseURL, "base-url", "", "Absolute page URL for --input-html")
	flags.BoolVarP(&help, "help", "h", false, "Show help")
	if err := flags.Parse(args); err != nil {
		return fail(diagnostics, err, 2)
	}
	if help {
		fmt.Fprintln(output, "Linkz collects JavaScript from explicitly supplied pages.\n\nUsage: linkz [options]\n       linkz diff BEFORE.jsonl AFTER.jsonl [options]")
		flags.SetOutput(output)
		flags.PrintDefaults()
		return 0
	}
	if showVersion {
		if _, err := fmt.Fprintln(output, "linkz "+buildVersion()); err != nil {
			return fail(diagnostics, err, 1)
		}
		return 0
	}
	if flags.NArg() != 0 {
		return fail(diagnostics, errors.New("use --url or stdin for page URLs"), 2)
	}
	if o.workers < 1 || o.workers > 150 {
		return fail(diagnostics, errors.New("workers must be between 1 and 150"), 2)
	}
	if o.timeout < 1 || o.timeout > 86400 {
		return fail(diagnostics, errors.New("timeout must be between 1 and 86400 seconds"), 2)
	}
	if o.maxSize < 1 || o.maxSize > 1<<30 {
		return fail(diagnostics, errors.New("max-size must be between 1 and 1073741824 bytes"), 2)
	}
	if (o.download || o.inline) && o.folder == "" {
		return fail(diagnostics, errors.New("download and save-inline require --folder"), 2)
	}
	if o.inputHTML != "" {
		if o.url != "" || o.download {
			return fail(diagnostics, errors.New("input-html cannot be combined with url or download"), 2)
		}
		if _, err := web.ParseURL(o.baseURL); err != nil {
			return fail(diagnostics, errors.New("input-html requires an absolute HTTP or HTTPS base-url"), 2)
		}
	} else if o.baseURL != "" {
		return fail(diagnostics, errors.New("base-url requires input-html"), 2)
	}
	if err := validatePaths(o); err != nil {
		return fail(diagnostics, err, 2)
	}
	if o.url != "" {
		if _, err := web.ParseURL(o.url); err != nil {
			return fail(diagnostics, err, 2)
		}
	}
	o.download = o.folder != "" && o.inputHTML == ""
	client, err := web.New(web.Config{Timeout: time.Duration(o.timeout) * time.Second, Proxy: o.proxy, Headers: o.headers, Redirects: o.redirects, MaxSize: o.maxSize, Workers: o.workers, AllowedOrigins: o.allowedOrigins})
	if err != nil {
		return fail(diagnostics, err, 2)
	}
	defer client.Close()
	var store *files.Store
	if o.folder != "" {
		store, err = files.NewStore(o.folder)
		if err != nil {
			return fail(diagnostics, err, 1)
		}
		defer store.Close()
	}
	var resultFile *os.File
	var manifestFile *os.File
	var manifest io.Writer
	if o.manifest != "" {
		manifestFile, err = os.Create(o.manifest)
		if err != nil {
			return fail(diagnostics, err, 1)
		}
		defer manifestFile.Close()
		manifest = manifestFile
	}
	if o.output != "" {
		resultFile, err = os.Create(o.output)
		if err != nil {
			return fail(diagnostics, err, 1)
		}
		defer resultFile.Close()
		output = io.MultiWriter(output, resultFile)
	}
	app := collector{options: o, client: client, store: store}
	err = app.collect(ctx, input, output, diagnostics, manifest)
	if resultFile != nil {
		err = errors.Join(err, resultFile.Close())
	}
	if manifestFile != nil {
		err = errors.Join(err, manifestFile.Close())
	}
	if err != nil {
		return fail(diagnostics, err, 1)
	}
	return 0
}

func fail(w io.Writer, err error, code int) int {
	fmt.Fprintf(w, "linkz: %v\n", err)
	return code
}
