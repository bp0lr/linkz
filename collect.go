package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
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
	URL         string
	File        string
	Error       string
	InlineIndex int
}

type pageResult struct {
	source, page string
	artifacts    []artifact
	err          error
}

func (c *collector) collect(ctx context.Context, input io.Reader, output, diagnostics io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan string)
	results := make(chan pageResult)
	inputErr := make(chan error, 1)
	go func() {
		defer close(jobs)
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
					result := c.process(ctx, job)
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
	errorsCount := 0
	printed := make(map[string]bool)
	var writeErr error
	for result := range results {
		if result.err != nil {
			fmt.Fprintf(diagnostics, "%s: %v\n", result.source, result.err)
			errorsCount++
			continue
		}
		if c.options.verbose {
			fmt.Fprintf(diagnostics, "%s: %d script records\n", result.source, len(result.artifacts))
		}
		for _, a := range result.artifacts {
			if a.Error != "" {
				fmt.Fprintf(diagnostics, "%s: %s\n", a.URL, a.Error)
				errorsCount++
			}
			if a.InlineIndex == 0 && !printed[a.URL] && writeErr == nil {
				printed[a.URL] = true
				_, writeErr = fmt.Fprintln(output, a.URL)
				if writeErr != nil {
					cancel()
				}
			}
		}
	}
	if writeErr != nil {
		return fmt.Errorf("write results: %w", writeErr)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := <-inputErr; err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	if errorsCount > 0 {
		return fmt.Errorf("%d collection errors", errorsCount)
	}
	return nil
}

func (c *collector) process(ctx context.Context, raw string) pageResult {
	result := pageResult{source: raw}
	u, err := web.ParseURL(raw)
	if err != nil {
		result.err = err
		return result
	}
	body, final, err := c.client.Get(ctx, u.String(), u.String())
	if err != nil {
		result.err = err
		return result
	}
	result.page = final
	scripts, err := extract(final, body)
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
			a.File, err = c.store.Save(final, block.index, block.code)
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
		entry.artifact.File, err = c.store.SaveReader(link, 0, resp.Body)
		resp.Body.Close()
	}
	if err != nil {
		entry.artifact.Error = err.Error()
	}
	close(entry.ready)
	return entry.artifact
}
