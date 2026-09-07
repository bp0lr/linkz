package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// Set with -ldflags "-X main.version=vX.Y.Z" when producing a release.
var version = "dev"

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
		var revision, dirty string
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				revision = setting.Value
			}
			if setting.Key == "vcs.modified" && setting.Value == "true" {
				dirty = "+dirty"
			}
		}
		if len(revision) > 12 {
			revision = revision[:12]
		}
		if revision != "" {
			return "dev (" + revision + dirty + ")"
		}
	}
	return version
}

// Reject conflicting inputs and outputs before opening any output with truncate.
func validatePaths(o options) error {
	paths := []string{o.output, o.manifest}
	if o.inputHTML != "" && o.inputHTML != "-" {
		paths = append(paths, o.inputHTML)
	}
	for i, a := range paths {
		if a == "" {
			continue
		}
		absA, err := filepath.Abs(a)
		if err != nil {
			return err
		}
		for _, b := range paths[i+1:] {
			if b == "" {
				continue
			}
			absB, err := filepath.Abs(b)
			if err != nil {
				return err
			}
			same := absA == absB || runtime.GOOS == "windows" && strings.EqualFold(absA, absB)
			infoA, errA := os.Stat(a)
			infoB, errB := os.Stat(b)
			if same || errA == nil && errB == nil && os.SameFile(infoA, infoB) {
				return fmt.Errorf("input-html, output and manifest must use different files")
			}
		}
	}
	return nil
}

type reporter struct {
	options             options
	output, diagnostics io.Writer
	manifest            *json.Encoder
	urls, files         map[string]bool
	pages, errors       int
	bytes               int64
	start               time.Time
}

func newReporter(o options, output, diagnostics, manifest io.Writer) *reporter {
	r := &reporter{options: o, output: output, diagnostics: diagnostics, urls: make(map[string]bool), files: make(map[string]bool), start: time.Now()}
	if manifest != nil {
		r.manifest = json.NewEncoder(manifest)
	}
	return r
}

func (r *reporter) write(page pageResult) error {
	r.pages++
	if page.err != nil {
		r.errors++
		fmt.Fprintf(r.diagnostics, "%s: %v\n", page.source, page.err)
		return r.record(artifact{SchemaVersion: 1, Source: page.source, Page: page.page, Kind: "page_error", Status: "error", Error: page.err.Error()})
	}
	if r.options.verbose {
		fmt.Fprintf(r.diagnostics, "%s: %d script records\n", page.source, len(page.artifacts))
	}
	for _, a := range page.artifacts {
		a.SchemaVersion, a.Source, a.Page = 1, page.source, page.page
		a.Kind, a.Status = "external", "listed"
		if a.InlineIndex > 0 {
			a.Kind = "inline"
		}
		if a.Error != "" {
			a.Status = "error"
			r.errors++
			fmt.Fprintf(r.diagnostics, "%s: %s\n", a.URL, a.Error)
		} else if a.File != "" {
			a.Status = "saved"
			if !r.files[a.File] {
				r.files[a.File] = true
				r.bytes += a.Size
			}
		}
		if a.InlineIndex == 0 && !r.urls[a.URL] {
			r.urls[a.URL] = true
			if _, err := fmt.Fprintln(r.output, a.URL); err != nil {
				return err
			}
		}
		if err := r.record(a); err != nil {
			return err
		}
	}
	return nil
}

func (r *reporter) record(a artifact) error {
	if r.manifest == nil {
		return nil
	}
	if err := r.manifest.Encode(a); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func (r *reporter) summarize() {
	if r.options.stats {
		fmt.Fprintf(r.diagnostics, "pages=%d urls=%d files=%d bytes=%d errors=%d elapsed=%s\n", r.pages, len(r.urls), len(r.files), r.bytes, r.errors, time.Since(r.start).Round(time.Millisecond))
	}
}
