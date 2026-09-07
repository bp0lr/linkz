package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConditionalReuseAndChangedContent(t *testing.T) {
	var revision, bodies, validations atomic.Int64
	revision.Store(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<script src="app.js"></script>`)
			return
		}
		tag := fmt.Sprintf(`"v%d"`, revision.Load())
		w.Header().Set("ETag", tag)
		if r.Header.Get("If-None-Match") == tag {
			validations.Add(1)
			w.WriteHeader(304)
			return
		}
		bodies.Add(1)
		fmt.Fprintf(w, "version=%d", revision.Load())
	}))
	defer server.Close()
	dir := t.TempDir()
	folder := filepath.Join(dir, "scripts")
	run := func(previous, name string) string {
		t.Helper()
		manifest := filepath.Join(dir, name)
		args := []string{"-u", server.URL, "-f", folder, "--manifest", manifest, "--stats"}
		if previous != "" {
			args = append(args, "--cache-from", previous)
		}
		code, _, diag := cli(t, "", args...)
		if code != 0 {
			t.Fatalf("%d %q", code, diag)
		}
		return manifest
	}
	first := run("", "first.jsonl")
	second := run(first, "second.jsonl")
	records := readManifest(t, second)
	if len(records) != 1 || !records[0].Reused || records[0].HTTPStatus != 304 || validations.Load() != 1 || bodies.Load() != 1 {
		t.Fatalf("records=%+v validations=%d bodies=%d", records, validations.Load(), bodies.Load())
	}
	code, out, diag := cli(t, "", "diff", first, second)
	if code != 0 || out != "" || !strings.Contains(diag, "unchanged=1") {
		t.Fatalf("diff=%d %q %q", code, out, diag)
	}
	revision.Store(2)
	third := run(second, "third.jsonl")
	records = readManifest(t, third)
	if records[0].Reused || records[0].ETag != `"v2"` || bodies.Load() != 2 {
		t.Fatalf("changed=%+v", records)
	}
	data, err := os.ReadFile(filepath.Join(folder, records[0].File))
	if err != nil || string(data) != "version=2" {
		t.Fatalf("%q %v", data, err)
	}
	code, out, _ = cli(t, "", "diff", second, third)
	if code != 0 || !strings.Contains(out, `"change":"changed"`) {
		t.Fatalf("%d %q", code, out)
	}
	if err := os.Remove(filepath.Join(folder, records[0].File)); err != nil {
		t.Fatal(err)
	}
	fourth := run(third, "fourth.jsonl")
	if readManifest(t, fourth)[0].Reused || bodies.Load() != 3 {
		t.Fatal("missing local file was reused")
	}
}

func TestConditionalLastModifiedAndOptOuts(t *testing.T) {
	for _, mode := range []string{"modified", "no-store", "vary", "headers", "no-validator"} {
		t.Run(mode, func(t *testing.T) {
			var conditional atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					fmt.Fprint(w, `<script src="app.js"></script>`)
					return
				}
				if mode != "no-validator" {
					w.Header().Set("Last-Modified", "Mon, 07 Sep 2026 10:00:00 GMT")
				}
				if mode == "no-store" {
					w.Header().Set("Cache-Control", "no-store")
				}
				if mode == "vary" {
					w.Header().Set("Vary", "X-Tenant")
				}
				if r.Header.Get("If-Modified-Since") != "" {
					conditional.Add(1)
					w.WriteHeader(304)
					return
				}
				fmt.Fprint(w, "app")
			}))
			defer server.Close()
			dir := t.TempDir()
			first, second := filepath.Join(dir, "first.jsonl"), filepath.Join(dir, "second.jsonl")
			base := []string{"-u", server.URL, "-f", filepath.Join(dir, "scripts")}
			if mode == "headers" {
				base = append(base, "-H", "X-Tenant: fixture")
			}
			code, _, diag := cli(t, "", append(append([]string{}, base...), "--manifest", first)...)
			if code != 0 {
				t.Fatalf("%d %q", code, diag)
			}
			code, _, diag = cli(t, "", append(append([]string{}, base...), "--cache-from", first, "--manifest", second, "--stats")...)
			if code != 0 {
				t.Fatalf("%d %q", code, diag)
			}
			want := mode == "modified"
			if readManifest(t, second)[0].Reused != want || (conditional.Load() == 1) != want {
				t.Fatalf("unexpected reuse in %s: %q", mode, diag)
			}
			if want && !strings.Contains(diag, "reused=1 downloaded_bytes=0") {
				t.Fatalf("stats=%q", diag)
			}
		})
	}
}

func TestCorruptCacheAndValidation(t *testing.T) {
	var conditional atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<script src="app.js"></script>`)
			return
		}
		w.Header().Set("ETag", `"fixed"`)
		if r.Header.Get("If-None-Match") != "" {
			conditional.Add(1)
			w.WriteHeader(304)
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer server.Close()
	dir := t.TempDir()
	folder := filepath.Join(dir, "scripts")
	first, second := filepath.Join(dir, "first.jsonl"), filepath.Join(dir, "second.jsonl")
	code, _, diag := cli(t, "", "-u", server.URL, "-f", folder, "--manifest", first)
	if code != 0 {
		t.Fatalf("%d %q", code, diag)
	}
	file := filepath.Join(folder, readManifest(t, first)[0].File)
	if err := os.WriteFile(file, []byte("oops"), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, diag = cli(t, "", "-u", server.URL, "-f", folder, "--cache-from", first, "--manifest", second)
	if code != 0 || conditional.Load() != 0 {
		t.Fatalf("%d %q", code, diag)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "good" {
		t.Fatalf("%q", data)
	}
	code, _, _ = cli(t, "", "--cache-from", first)
	if code != 2 {
		t.Fatalf("missing folder=%d", code)
	}
	code, _, _ = cli(t, "", "--cache-from", first, "-f", folder, "--manifest", first)
	if code != 2 {
		t.Fatalf("overlap=%d", code)
	}
	code, _, _ = cli(t, "", "--cache-from", first, "-f", folder, "--input-html", "-", "--base-url", server.URL)
	if code != 2 {
		t.Fatalf("offline=%d", code)
	}
}

func TestCDNRevalidationRecoversFileRemovedDuring304(t *testing.T) {
	var cachedFile atomic.Value
	var bodies, validations atomic.Int64
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant") != "" {
			t.Error("custom page header reached CDN")
		}
		w.Header().Set("ETag", `"cdn-v1"`)
		if r.Header.Get("If-None-Match") == `"cdn-v1"` {
			validations.Add(1)
			if err := os.Remove(cachedFile.Load().(string)); err != nil {
				t.Error(err)
			}
			w.WriteHeader(304)
			return
		}
		bodies.Add(1)
		fmt.Fprint(w, "cdn content")
	}))
	defer cdn.Close()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<script src="%s/app.js"></script>`, cdn.URL)
	}))
	defer page.Close()
	dir := t.TempDir()
	folder := filepath.Join(dir, "scripts")
	first, second := filepath.Join(dir, "first.jsonl"), filepath.Join(dir, "second.jsonl")
	base := []string{"-u", page.URL, "--allow-origin", cdn.URL, "-H", "X-Tenant: fixture", "-f", folder}
	code, _, diag := cli(t, "", append(append([]string{}, base...), "--manifest", first)...)
	if code != 0 {
		t.Fatalf("%d %q", code, diag)
	}
	cachedFile.Store(filepath.Join(folder, readManifest(t, first)[0].File))
	code, _, diag = cli(t, "", append(append([]string{}, base...), "--cache-from", first, "--manifest", second)...)
	if code != 0 || validations.Load() != 1 || bodies.Load() != 2 {
		t.Fatalf("code=%d validations=%d bodies=%d %q", code, validations.Load(), bodies.Load(), diag)
	}
	a := readManifest(t, second)[0]
	data, err := os.ReadFile(filepath.Join(folder, a.File))
	if err != nil || string(data) != "cdn content" || a.Reused || a.HTTPStatus != 200 {
		t.Fatalf("artifact=%+v data=%q err=%v", a, data, err)
	}
}
