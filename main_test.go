package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeduplicatePagesAndSharedDownloads(t *testing.T) {
	var pages, scripts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app.js" {
			scripts.Add(1)
			fmt.Fprint(w, "app")
			return
		}
		pages.Add(1)
		fmt.Fprint(w, `<script src="/app.js"></script>`)
	}))
	defer server.Close()
	input := server.URL + "/one\n" + server.URL + "/one#fragment\n" + server.URL + "/two\n"
	code, out, diag := cli(t, input, "-f", t.TempDir(), "-w", "3")
	if code != 0 || pages.Load() != 2 || scripts.Load() != 1 || out != server.URL+"/app.js\n" {
		t.Fatalf("code=%d pages=%d scripts=%d out=%q diag=%q", code, pages.Load(), scripts.Load(), out, diag)
	}
}

func cli(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, diag bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &out, &diag)
	return code, out.String(), diag.String()
}

func TestListAndOutputWithoutDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Error("listing downloaded a script")
		}
		fmt.Fprint(w, `<script src="app.js?v=1"></script><script src="app.js?v=1"></script>`)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "urls.txt")
	os.WriteFile(file, []byte("old data"), 0600)
	code, out, diag := cli(t, "", "-u", server.URL, "-o", file)
	if code != 0 || out != server.URL+"/app.js?v=1\n" || diag != "" {
		t.Fatalf("code=%d out=%q diag=%q", code, out, diag)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != out {
		t.Fatalf("file=%q err=%v", data, err)
	}
}

func TestCLIValidation(t *testing.T) {
	for _, args := range [][]string{{"-w", "0"}, {"--timeout", "0"}, {"--max-size", "0"}, {"-s"}, {"-d"}, {"--unknown"}, {"positional"}, {"-H", "broken"}, {"-u", "relative/path"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, out, diag := cli(t, "", args...)
			if code != 2 || out != "" || diag == "" {
				t.Fatalf("code=%d out=%q diag=%q", code, out, diag)
			}
		})
	}
	code, out, diag := cli(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage:") || diag != "" {
		t.Fatalf("help: %d %q %q", code, out, diag)
	}
}

func TestSaveScriptsAndInline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<script src="a/app.js?v=1"></script><script src="b/app.js?v=1"></script><script src="a/app.js?v=2"></script><script>const x = 1;</script><script type="application/json">{"x":1}</script>`)
		} else {
			fmt.Fprintf(w, "// %s", r.URL.String())
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	code, out, diag := cli(t, "", "-u", server.URL, "-f", dir, "-s")
	if code != 0 || len(strings.Fields(out)) != 3 || diag != "" {
		t.Fatalf("%d %q %q", code, out, diag)
	}
	contents := make(map[string]bool)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		contents[string(data)] = true
		return err
	})
	if err != nil || len(contents) != 4 || !contents["const x = 1;"] || !contents["// /a/app.js?v=2"] {
		t.Fatalf("contents=%v err=%v", contents, err)
	}
}

func TestPartialFailureAndInputError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<script src="ok.js"></script><script src="missing.js"></script>`)
			return
		}
		if r.URL.Path == "/missing.js" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()
	code, out, diag := cli(t, "", "-u", server.URL, "-f", t.TempDir())
	if code != 1 || len(strings.Fields(out)) != 2 || !strings.Contains(diag, "404") {
		t.Fatalf("%d %q %q", code, out, diag)
	}
	code, _, diag = cli(t, strings.Repeat("a", 1<<20))
	if code != 1 || !strings.Contains(diag, "read input") {
		t.Fatalf("%d %q", code, diag)
	}
}

func TestRedirectUsesFinalPageAsBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/nested/index.html", http.StatusFound)
			return
		}
		fmt.Fprint(w, `<script src="app.js"></script>`)
	}))
	defer server.Close()
	code, out, diag := cli(t, "", "-u", server.URL, "--follow-redirect")
	if code != 0 || out != server.URL+"/nested/app.js\n" {
		t.Fatalf("%d %q %q", code, out, diag)
	}
}
