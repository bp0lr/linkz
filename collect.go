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
	options      options
	client       *web.Client
	store        *files.Store
	downloadMu   sync.Mutex
	downloads    map[string]*downloadEntry
	downloadJobs chan downloadTask
	previous     map[string]artifact
}

type downloadTask struct {
	link, origin string
	entry        *downloadEntry
}

type downloadEntry struct {
	ready    chan struct{}
	artifact artifact
}

type artifact struct {
	SchemaVersion int    `json:"schema_version"`
	Source        string `json:"source"`
	Origin        string `json:"origin,omitempty"`
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
	ETag          string `json:"etag,omitempty"`
	LastModified  string `json:"last_modified,omitempty"`
	Cacheable     bool   `json:"cacheable,omitempty"`
	Reused        bool   `json:"reused,omitempty"`
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
	var downloadWorkers sync.WaitGroup
	if c.options.download {
		c.downloadJobs = make(chan downloadTask, c.options.workers)
		for i := 0; i < c.options.workers; i++ {
			downloadWorkers.Add(1)
			go func() {
				defer downloadWorkers.Done()
				for task := range c.downloadJobs {
					c.fetchResource(ctx, task)
				}
			}()
		}
	}
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
	go func() {
		workers.Wait()
		if c.downloadJobs != nil {
			close(c.downloadJobs)
			downloadWorkers.Wait()
		}
		close(results)
	}()
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
	scope, err := c.client.Scope(raw)
	if err != nil {
		result.err = err
		return result
	}
	result.page = u.String()
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
	scripts, err := extract(final, body, c.options.includeLibs, scope)
	if err != nil {
		result.err = err
		return result
	}
	pending := make([]*downloadEntry, len(scripts.links))
	if c.options.download {
		for i, link := range scripts.links {
			pending[i] = c.scheduleDownload(ctx, link, web.Origin(u))
		}
	}
	for i, link := range scripts.links {
		a := artifact{URL: link, Origin: web.Origin(u)}
		if c.options.download {
			select {
			case <-pending[i].ready:
				a = pending[i].artifact
			case <-ctx.Done():
				a.Error = ctx.Err().Error()
			}
		}
		result.artifacts = append(result.artifacts, a)
	}
	if c.options.inline {
		for _, block := range scripts.inline {
			a := artifact{URL: final, Origin: web.Origin(u), InlineIndex: block.index}
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
func (c *collector) scheduleDownload(ctx context.Context, link, origin string) *downloadEntry {
	key := origin + "\x00" + link
	c.downloadMu.Lock()
	if c.downloads == nil {
		c.downloads = make(map[string]*downloadEntry)
	}
	if entry, exists := c.downloads[key]; exists {
		c.downloadMu.Unlock()
		return entry
	}
	entry := &downloadEntry{ready: make(chan struct{}), artifact: artifact{URL: link, Origin: origin}}
	c.downloads[key] = entry
	c.downloadMu.Unlock()
	select {
	case c.downloadJobs <- downloadTask{link, origin, entry}:
	case <-ctx.Done():
		entry.artifact.Error = ctx.Err().Error()
		close(entry.ready)
	}
	return entry
}

func (c *collector) fetchResource(ctx context.Context, task downloadTask) {
	entry := task.entry
	defer close(entry.ready)
	if previous, exists := c.previous[task.origin+"\x00"+task.link]; exists && c.client.CanRevalidate(task.link, task.origin) && c.store.Verify(savedArtifact(previous), c.options.maxSize) {
		resp, err := c.client.OpenConditional(ctx, task.link, task.origin, web.Validators{ETag: previous.ETag, LastModified: previous.LastModified})
		if err != nil {
			entry.artifact.Error = err.Error()
			return
		}
		if resp.Status == 304 {
			resp.Body.Close()
			if c.store.Verify(savedArtifact(previous), c.options.maxSize) {
				entry.artifact.File, entry.artifact.Size, entry.artifact.SHA256 = previous.File, previous.Size, previous.SHA256
				entry.artifact.FinalURL, entry.artifact.HTTPStatus, entry.artifact.Reused = resp.URL, resp.Status, true
				entry.artifact.Cacheable = resp.Cacheable
				entry.artifact.ETag, entry.artifact.LastModified = previous.ETag, previous.LastModified
				if resp.Validators.ETag != "" {
					entry.artifact.ETag = resp.Validators.ETag
				}
				if resp.Validators.LastModified != "" {
					entry.artifact.LastModified = resp.Validators.LastModified
				}
				return
			}
		} else {
			c.saveResponse(task, resp)
			return
		}
	}
	resp, err := c.client.Open(ctx, task.link, task.origin)
	if err != nil {
		entry.artifact.Error = err.Error()
		return
	}
	c.saveResponse(task, resp)
}

func (c *collector) saveResponse(task downloadTask, resp *web.Response) {
	defer resp.Body.Close()
	a := &task.entry.artifact
	a.FinalURL, a.HTTPStatus = resp.URL, resp.Status
	a.ETag, a.LastModified, a.Cacheable = resp.Validators.ETag, resp.Validators.LastModified, resp.Cacheable
	saved, err := c.store.SaveResource(task.link, task.origin, resp.Body)
	a.File, a.Size, a.SHA256 = saved.File, saved.Size, saved.SHA256
	if err != nil {
		a.Error = err.Error()
	}
}
