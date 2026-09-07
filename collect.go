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
	options options
	client  *web.Client
	store   *files.Store
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
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
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
			if a.InlineIndex == 0 && writeErr == nil {
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
			data, _, err := c.client.Get(ctx, link, final)
			if err == nil {
				a.File, err = c.store.Save(link, 0, data)
			}
			if err != nil {
				a.Error = err.Error()
			}
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
