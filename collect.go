package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	web "github.com/bp0lr/linkz/fetch"
	files "github.com/bp0lr/linkz/fileutils"
)

type collector struct {
	options    options
	client     *web.Client
	store      *files.Store
	downloadMu sync.Mutex
	downloads  map[string]*downloadEntry
}

type downloadEntry struct {
	ready    chan struct{}
	artifact artifact
}

type artifact struct {
	SchemaVersion int    `json:"schema_version"`
	Source        string `json:"source"`
	Page          string `json:"page,omitempty"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	URL           string `json:"url,omitempty"`
	FinalURL      string `json:"final_url,omitempty"`
	HTTPStatus    int    `json:"http_status,omitempty"`
	File          string `json:"file,omitempty"`
	Size          int64  `json:"size_bytes"`
	SHA256        string `json:"sha256,omitempty"`
	Error         string `json:"error,omitempty"`
	InlineIndex   int    `json:"inline_index,omitempty"`
}

type pageResult struct {
	source, page string
	artifacts    []artifact
	err          error
}

func (c *collector) collect(ctx context.Context, input io.Reader, output, diagnostics, manifest io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan string)
	results := make(chan pageResult)
	inputErr := make(chan error, 1)
	go func() {
		defer close(jobs)
		if c.options.inputHTML != "" {
			select {
			case jobs <- c.options.baseURL:
				inputErr <- nil
			case <-ctx.Done():
				inputErr <- ctx.Err()
			}
			return
		}
		if c.options.url != "" {
			input = strings.NewReader(c.options.url)
		}
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		seen := make(map[string]bool)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if u, err := web.ParseURL(line); err == nil {
				line = u.String()
			}
			if seen[line] {
				continue
			}
			seen[line] = true
			select {
			case jobs <- line:
			case <-ctx.Done():
				inputErr <- ctx.Err()
				return
			}
		}
		inputErr <- scanner.Err()
	}()
	var workers sync.WaitGroup
	for i := 0; i < c.options.workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					result := c.process(ctx, job, input)
					select {
					case results <- result:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() { workers.Wait(); close(results) }()
	report := newReporter(c.options, output, diagnostics, manifest)
	defer report.summarize()
	var writeErr error
	for result := range results {
		if writeErr == nil {
			writeErr = report.write(result)
			if writeErr != nil {
				report.errors++
				cancel()
			}
		}
	}
	if writeErr != nil {
		return fmt.Errorf("write results: %w", writeErr)
	}
	if ctx.Err() != nil {
		report.errors++
		return ctx.Err()
	}
	if err := <-inputErr; err != nil {
		report.errors++
		return fmt.Errorf("read input: %w", err)
	}
	if report.errors > 0 {
		return fmt.Errorf("%d collection errors", report.errors)
	}
	return nil
}

func (c *collector) process(ctx context.Context, raw string, input io.Reader) pageResult {
	result := pageResult{source: raw}
	u, err := web.ParseURL(raw)
	if err != nil {
		result.err = err
		return result
	}
	var body []byte
	final := u.String()
	if c.options.inputHTML != "" {
		result.source = c.options.inputHTML
		if c.options.inputHTML != "-" {
			file, openErr := os.Open(c.options.inputHTML)
			if openErr != nil {
				result.err = openErr
				return result
			}
			defer file.Close()
			input = file
		}
		body, err = io.ReadAll(io.LimitReader(input, c.options.maxSize+1))
		if err == nil && int64(len(body)) > c.options.maxSize {
			err = web.ErrTooLarge
		}
	} else {
		body, final, err = c.client.Get(ctx, u.String(), u.String())
	}
	if err != nil {
		result.err = err
		return result
	}
	result.page = final
	scripts, err := extract(final, body, c.options.includeLibs)
	if err != nil {
		result.err = err
		return result
	}
	for _, link := range scripts.links {
		a := artifact{URL: link}
		if c.options.download {
			a = c.download(ctx, link, final)
		}
		result.artifacts = append(result.artifacts, a)
	}
	if c.options.inline {
		for _, block := range scripts.inline {
			a := artifact{URL: final, InlineIndex: block.index}
			var saved files.Saved
			saved, err = c.store.Save(final, block.index, block.code)
			a.File, a.Size, a.SHA256 = saved.File, saved.Size, saved.SHA256
			if err != nil {
				a.Error = err.Error()
			}
			result.artifacts = append(result.artifacts, a)
		}
	}
	return result
}

// Cache both successes and failures for this run. Concurrent pages referencing
// the same URL wait for one download while retaining their own provenance.
func (c *collector) download(ctx context.Context, link, origin string) artifact {
	c.downloadMu.Lock()
	if c.downloads == nil {
		c.downloads = make(map[string]*downloadEntry)
	}
	if entry, exists := c.downloads[link]; exists {
		c.downloadMu.Unlock()
		select {
		case <-entry.ready:
			return entry.artifact
		case <-ctx.Done():
			return artifact{URL: link, Error: ctx.Err().Error()}
		}
	}
	entry := &downloadEntry{ready: make(chan struct{}), artifact: artifact{URL: link}}
	c.downloads[link] = entry
	c.downloadMu.Unlock()
	resp, err := c.client.Open(ctx, link, origin)
	if err == nil {
		entry.artifact.FinalURL, entry.artifact.HTTPStatus = resp.URL, resp.Status
		var saved files.Saved
		saved, err = c.store.SaveReader(link, 0, resp.Body)
		entry.artifact.File, entry.artifact.Size, entry.artifact.SHA256 = saved.File, saved.Size, saved.SHA256
		resp.Body.Close()
	}
	if err != nil {
		entry.artifact.Error = err.Error()
	}
	close(entry.ready)
	return entry.artifact
}
