package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/pflag"
)

type difference struct {
	Change      string    `json:"change"`
	Source      string    `json:"source"`
	Kind        string    `json:"kind"`
	URL         string    `json:"url,omitempty"`
	InlineIndex int       `json:"inline_index,omitempty"`
	Before      *artifact `json:"before,omitempty"`
	After       *artifact `json:"after,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

func runDiff(args []string, output, diagnostics io.Writer) int {
	var file string
	var help, all bool
	flags := pflag.NewFlagSet("diff", pflag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.StringVarP(&file, "output", "o", "", "Write JSONL differences to a file (replace existing contents)")
	flags.BoolVar(&all, "all", false, "Include unchanged records")
	flags.BoolVarP(&help, "help", "h", false, "Show diff help")
	if err := flags.Parse(args); err != nil {
		return fail(diagnostics, err, 2)
	}
	if help {
		fmt.Fprintln(output, "Usage: linkz diff BEFORE.jsonl AFTER.jsonl [--all] [-o changes.jsonl]")
		return 0
	}
	if flags.NArg() != 2 {
		return fail(diagnostics, fmt.Errorf("diff requires two inventory files"), 2)
	}
	for _, input := range flags.Args() {
		if err := validatePaths(options{output: file, inputHTML: input}); err != nil {
			return fail(diagnostics, err, 2)
		}
	}
	before, err := loadInventory(flags.Arg(0))
	if err != nil {
		return fail(diagnostics, fmt.Errorf("before: %w", err), 1)
	}
	after, err := loadInventory(flags.Arg(1))
	if err != nil {
		return fail(diagnostics, fmt.Errorf("after: %w", err), 1)
	}
	var resultFile *os.File
	if file != "" {
		resultFile, err = os.Create(file)
		if err != nil {
			return fail(diagnostics, err, 1)
		}
		defer resultFile.Close()
		output = io.MultiWriter(output, resultFile)
	}
	encoder := json.NewEncoder(output)
	counts := make(map[string]int)
	for _, d := range compareInventories(before, after) {
		counts[d.Change]++
		if !all && d.Change == "unchanged" {
			continue
		}
		if err := encoder.Encode(d); err != nil {
			return fail(diagnostics, err, 1)
		}
	}
	if resultFile != nil {
		if err := resultFile.Close(); err != nil {
			return fail(diagnostics, err, 1)
		}
	}
	fmt.Fprintf(diagnostics, "added=%d removed=%d changed=%d unchanged=%d unknown=%d\n", counts["added"], counts["removed"], counts["changed"], counts["unchanged"], counts["unknown"])
	return 0
}

func compareInventories(before, after *inventory) []difference {
	keys := make(map[artifactKey]bool)
	for k := range before.artifacts {
		keys[k] = true
	}
	for k := range after.artifacts {
		keys[k] = true
	}
	ordered := make([]artifactKey, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.URL != b.URL {
			return a.URL < b.URL
		}
		return a.Index < b.Index
	})
	var differences []difference
	uncertain := make(map[string]bool)
	for _, key := range ordered {
		old, had := before.artifacts[key]
		current, has := after.artifacts[key]
		d := difference{Source: key.Source, Kind: key.Kind, URL: key.URL, InlineIndex: key.Index}
		if had {
			d.Before = &old
		}
		if has {
			d.After = &current
		}
		switch {
		case !had:
			p := before.pages[key.Source]
			if p.complete && !p.failed {
				d.Change = "added"
			} else {
				d.Change, d.Reason = "unknown", "page not confirmed complete in the earlier inventory"
			}
		case !has:
			p := after.pages[key.Source]
			if p.complete && !p.failed {
				d.Change = "removed"
			} else {
				d.Change, d.Reason = "unknown", "page missing, incomplete or failed in the current inventory"
			}
		case old.Status != "saved" || current.Status != "saved" || before.pages[key.Source].failed || after.pages[key.Source].failed:
			d.Change, d.Reason = "unknown", "content hashes cannot establish a successful comparison"
		case old.SHA256 == current.SHA256:
			d.Change = "unchanged"
		default:
			d.Change = "changed"
		}
		differences = append(differences, d)
		if d.Change == "unknown" {
			uncertain[key.Source] = true
		}
	}
	pages := make(map[string]bool)
	for source := range before.pages {
		pages[source] = true
	}
	for source := range after.pages {
		pages[source] = true
	}
	sources := make([]string, 0, len(pages))
	for source := range pages {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	for _, source := range sources {
		a, b := before.pages[source], after.pages[source]
		if !uncertain[source] && (a.failed || b.failed || !a.complete || !b.complete) {
			differences = append(differences, difference{Change: "unknown", Source: source, Kind: "page", Reason: "page completeness cannot be established in both inventories"})
		}
	}
	return differences
}
