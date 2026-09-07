package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowedCDNRepresentationsKeepTheirContext(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			fmt.Fprint(w, `<script src="app.js"></script>`)
			return
		}
		if r.Header.Get("X-Api-Key") == "fixture" {
			fmt.Fprint(w, "private")
		} else {
			fmt.Fprint(w, "public")
		}
	}))
	defer cdn.Close()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<script src="%s/app.js"></script>`, cdn.URL)
	}))
	defer page.Close()
	code, out, diag := cli(t, "", "-u", page.URL)
	if code != 0 || out != "" {
		t.Fatalf("default scope: %d %q %q", code, out, diag)
	}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.jsonl")
	folder := filepath.Join(dir, "scripts")
	code, out, diag = cli(t, page.URL+"\n"+cdn.URL+"/page\n", "--allow-origin", cdn.URL, "-H", "X-Api-Key: fixture", "-f", folder, "--manifest", manifest)
	if code != 0 || out != cdn.URL+"/app.js\n" {
		t.Fatalf("%d %q %q", code, out, diag)
	}
	records := readManifest(t, manifest)
	if len(records) != 2 || records[0].File == records[1].File || records[0].Origin == records[1].Origin {
		t.Fatalf("mixed contexts: %+v", records)
	}
	contents := map[string]bool{}
	for _, record := range records {
		data, err := os.ReadFile(filepath.Join(folder, record.File))
		if err != nil {
			t.Fatal(err)
		}
		contents[string(data)] = true
	}
	if !contents["private"] || !contents["public"] {
		t.Fatalf("representations=%v", contents)
	}
}

func TestPageRedirectKeepsOriginalScope(t *testing.T) {
	var original string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<script src="%s/app.js"></script><script src="cdn.js"></script>`, original)
	}))
	defer cdn.Close()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, cdn.URL+"/page", http.StatusFound) }))
	defer page.Close()
	original = page.URL
	code, out, diag := cli(t, "", "-u", page.URL, "--allow-origin", cdn.URL, "--follow-redirect")
	if code != 0 || !strings.Contains(out, page.URL+"/app.js\n") || !strings.Contains(out, cdn.URL+"/cdn.js\n") {
		t.Fatalf("%d %q %q", code, out, diag)
	}
}
