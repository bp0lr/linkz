package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureManifest(t *testing.T, records ...artifact) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "inventory.jsonl")
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range records {
		if err := json.NewEncoder(f).Encode(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

func savedRecord(name, hash string) artifact {
	return artifact{SchemaVersion: manifestVersion, Source: "https://example.test/", Page: "https://example.test/", URL: "https://example.test/" + name, Kind: "external", Status: "saved", File: name, SHA256: strings.Repeat(hash, 64), Size: 10}
}

func completeRecord() artifact {
	return artifact{SchemaVersion: manifestVersion, Source: "https://example.test/", Page: "https://example.test/", Kind: "page", Status: "processed"}
}

func TestDiffChanges(t *testing.T) {
	oldInline := savedRecord("", "a")
	oldInline.Kind, oldInline.InlineIndex, oldInline.File = "inline", 1, "old-inline.js"
	newInline := oldInline
	newInline.SHA256, newInline.File = strings.Repeat("b", 64), "new-inline.js"
	before := fixtureManifest(t, savedRecord("app.js", "a"), savedRecord("gone.js", "a"), savedRecord("same.js", "a"), oldInline, completeRecord())
	after := fixtureManifest(t, newInline, savedRecord("same.js", "a"), savedRecord("new.js", "a"), savedRecord("app.js", "b"), completeRecord())
	file := filepath.Join(t.TempDir(), "changes.jsonl")
	code, out, diag := cli(t, "", "diff", before, after, "-o", file)
	if code != 0 || !strings.Contains(diag, "added=1 removed=1 changed=2 unchanged=1 unknown=0") {
		t.Fatalf("%d %q %q", code, out, diag)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("%q", out)
	}
	for _, line := range lines {
		var d difference
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatal(err)
		}
		if d.Change == "unchanged" {
			t.Fatal("unexpected unchanged output")
		}
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != out {
		t.Fatalf("output: %q %v", data, err)
	}
	code, all, _ := cli(t, "", "diff", before, after, "--all")
	if code != 0 || len(strings.Split(strings.TrimSpace(all), "\n")) != 5 {
		t.Fatalf("all=%q", all)
	}
}

func TestDiffMissingAndFailedPagesAreUnknown(t *testing.T) {
	before := fixtureManifest(t, savedRecord("app.js", "a"), completeRecord())
	failure := completeRecord()
	failure.Kind, failure.Status, failure.Error = "page_error", "error", "request failed"
	for _, after := range []string{fixtureManifest(t, failure), fixtureManifest(t)} {
		code, out, diag := cli(t, "", "diff", before, after)
		if code != 0 || !strings.Contains(out, `"change":"unknown"`) || !strings.Contains(diag, "removed=0") {
			t.Fatalf("%d %q %q", code, out, diag)
		}
	}
	code, out, _ := cli(t, "", "diff", before, fixtureManifest(t, completeRecord()))
	if code != 0 || !strings.Contains(out, `"change":"removed"`) {
		t.Fatalf("empty complete page: %d %q", code, out)
	}
	code, out, _ = cli(t, "", "diff", fixtureManifest(t, completeRecord()), fixtureManifest(t, failure))
	if code != 0 || !strings.Contains(out, `"kind":"page"`) {
		t.Fatalf("empty failed page: %d %q", code, out)
	}
}

func TestDiffLegacyAndListedRecords(t *testing.T) {
	old := savedRecord("app.js", "a")
	old.SchemaVersion = 1
	before := fixtureManifest(t, old)
	after := fixtureManifest(t, savedRecord("app.js", "b"), completeRecord())
	code, out, diag := cli(t, "", "diff", before, after)
	if code != 0 || !strings.Contains(out, `"change":"changed"`) || !strings.Contains(diag, "unknown=1") {
		t.Fatalf("%d %q %q", code, out, diag)
	}
	listed := old
	listed.Status, listed.SHA256, listed.File = "listed", "", ""
	code, out, _ = cli(t, "", "diff", fixtureManifest(t, listed), after)
	if code != 0 || !strings.Contains(out, `"change":"unknown"`) {
		t.Fatalf("listed: %d %q", code, out)
	}
}

func TestDiffValidationAndOutputProtection(t *testing.T) {
	good := fixtureManifest(t, completeRecord())
	code, _, _ := cli(t, "", "diff", good)
	if code != 2 {
		t.Fatalf("arity=%d", code)
	}
	code, _, _ = cli(t, "", "diff", good, good, "-o", good)
	if code != 2 {
		t.Fatalf("overlap=%d", code)
	}
	bad := savedRecord("app.js", "a")
	bad.SchemaVersion = 99
	conflict := savedRecord("app.js", "b")
	for _, name := range []string{fixtureManifest(t, bad), fixtureManifest(t, savedRecord("app.js", "a"), conflict)} {
		code, _, _ = cli(t, "", "diff", good, name)
		if code != 1 {
			t.Fatalf("invalid input accepted: %d", code)
		}
	}
	code, out, diag := cli(t, "", "diff", good, good)
	if code != 0 || out != "" || !strings.Contains(diag, "unknown=0") {
		t.Fatalf("same input: %d %q %q", code, out, diag)
	}
}
