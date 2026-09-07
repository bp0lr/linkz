package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func readManifest(t *testing.T, name string) []artifact {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	var records []artifact
	for {
		var record artifact
		if err := decoder.Decode(&record); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func TestManifestPreservesProvenanceAndHashes(t *testing.T) {
	var downloads atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app.js" {
			downloads.Add(1)
			fmt.Fprint(w, "const app = 1;")
			return
		}
		fmt.Fprint(w, `<script src="/app.js"></script>`)
	}))
	defer server.Close()
	dir := t.TempDir()
	folder := filepath.Join(dir, "scripts")
	manifest := filepath.Join(dir, "manifest.jsonl")
	input := server.URL + "/one\n" + server.URL + "/two\n"
	code, out, diag := cli(t, input, "-f", folder, "--manifest", manifest, "--stats")
	if code != 0 || out != server.URL+"/app.js\n" || downloads.Load() != 1 || !strings.Contains(diag, "pages=2 urls=1 files=1 bytes=14 errors=0") {
		t.Fatalf("%d %q %q downloads=%d", code, out, diag, downloads.Load())
	}
	records := readManifest(t, manifest)
	if len(records) != 2 || records[0].Source == records[1].Source {
		t.Fatalf("provenance: %+v", records)
	}
	for _, record := range records {
		data, err := os.ReadFile(filepath.Join(folder, record.File))
		if err != nil {
			t.Fatal(err)
		}
		if record.SchemaVersion != 1 || record.Kind != "external" || record.Status != "saved" || record.Page != record.Source || record.Size != int64(len(data)) || record.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || record.HTTPStatus != 200 || record.FinalURL != record.URL {
			t.Fatalf("record=%+v", record)
		}
	}
	if records[0].File != records[1].File {
		t.Fatal("shared resource has different paths")
	}
}

func TestLocalHTMLNeverUsesNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("offline mode made an HTTP request") }))
	defer server.Close()
	for _, stdin := range []bool{false, true} {
		t.Run(fmt.Sprint(stdin), func(t *testing.T) {
			dir := t.TempDir()
			manifest := filepath.Join(dir, "manifest.jsonl")
			folder := filepath.Join(dir, "scripts")
			html := `<script src="app.js"></script><script src="jquery.js"></script><script type="module">export {};</script>`
			source := filepath.Join(dir, "page.html")
			if stdin {
				source = "-"
			} else if err := os.WriteFile(source, []byte(html), 0600); err != nil {
				t.Fatal(err)
			}
			code, out, diag := cli(t, html, "--input-html", source, "--base-url", server.URL+"/page.html", "-f", folder, "-s", "--include-libs", "--manifest", manifest, "--stats")
			if code != 0 || len(strings.Fields(out)) != 2 || !strings.Contains(diag, "pages=1 urls=2 files=1 bytes=10 errors=0") {
				t.Fatalf("%d %q %q", code, out, diag)
			}
			records := readManifest(t, manifest)
			if len(records) != 3 {
				t.Fatalf("records=%+v", records)
			}
			for _, record := range records {
				if record.Source != source {
					t.Errorf("source=%q", record.Source)
				}
				if record.Kind == "inline" {
					data, err := os.ReadFile(filepath.Join(folder, record.File))
					if err != nil || string(data) != "export {};" || record.Status != "saved" || record.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
						t.Fatalf("%+v data=%q err=%v", record, data, err)
					}
				} else if record.Status != "listed" || record.File != "" || record.SHA256 != "" {
					t.Fatalf("unexpected external download: %+v", record)
				}
			}
		})
	}
}

func TestIncludeLibrariesIsOptIn(t *testing.T) {
	html := `<script src="jquery.min.js?v=2"></script>`
	args := []string{"--input-html", "-", "--base-url", "https://example.test/"}
	code, out, diag := cli(t, html, args...)
	if code != 0 || out != "" || diag != "" {
		t.Fatalf("%d %q %q", code, out, diag)
	}
	code, out, diag = cli(t, html, append(args, "--include-libs")...)
	if code != 0 || out != "https://example.test/jquery.min.js?v=2\n" {
		t.Fatalf("%d %q %q", code, out, diag)
	}
}

func TestInventoryValidationAndLocalLimit(t *testing.T) {
	for _, args := range [][]string{{"--input-html", "-"}, {"--base-url", "https://example.test/"}, {"--input-html", "-", "--base-url", "https://example.test/", "-u", "https://example.test/"}, {"--input-html", "-", "--base-url", "https://example.test/", "-d", "-f", t.TempDir()}} {
		code, _, diag := cli(t, "", args...)
		if code != 2 {
			t.Fatalf("args=%v code=%d diag=%q", args, code, diag)
		}
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "page.html")
	if err := os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, diag := cli(t, "", "--input-html", source, "--base-url", "https://example.test/", "--manifest", source)
	if code != 2 {
		t.Fatalf("%d %q", code, diag)
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "original" {
		t.Fatalf("input changed: %q %v", data, err)
	}
	code, _, diag = cli(t, "", "-o", source, "--manifest", filepath.Join(dir, ".", "page.html"))
	if code != 2 {
		t.Fatalf("output collision: %d %q", code, diag)
	}
	manifest := filepath.Join(dir, "manifest.jsonl")
	code, _, diag = cli(t, "12345", "--input-html", "-", "--base-url", "https://example.test/", "--max-size", "4", "--manifest", manifest)
	if code != 1 || !strings.Contains(diag, "max-size") {
		t.Fatalf("%d %q", code, diag)
	}
	records := readManifest(t, manifest)
	if len(records) != 1 || records[0].Kind != "page_error" || records[0].Status != "error" {
		t.Fatalf("%+v", records)
	}
}

func TestVersionAndLegacyStats(t *testing.T) {
	code, out, diag := cli(t, "", "--version")
	if code != 0 || !strings.HasPrefix(out, "linkz ") || diag != "" {
		t.Fatalf("%d %q %q", code, out, diag)
	}
	code, out, diag = cli(t, "", "--use-pb")
	if code != 0 || out != "" || !strings.Contains(diag, "pages=0 urls=0 files=0") {
		t.Fatalf("%d %q %q", code, out, diag)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestWriterFailures(t *testing.T) {
	var diag bytes.Buffer
	code := run(context.Background(), []string{"--input-html", "-", "--base-url", "https://example.test/"}, strings.NewReader(`<script src="app.js"></script>`), brokenWriter{}, &diag)
	if code != 1 || !strings.Contains(diag.String(), "output unavailable") {
		t.Fatalf("%d %q", code, diag.String())
	}
	r := newReporter(options{}, io.Discard, io.Discard, brokenWriter{})
	if err := r.write(pageResult{source: "test", artifacts: []artifact{{URL: "https://example.test/app.js"}}}); err == nil {
		t.Fatal("ignored manifest error")
	}
}
