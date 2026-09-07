package fileutils

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmptyHashAndInlineContentIdentity(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	empty, err := s.Save("https://example.test/empty.js", 0, nil)
	if err != nil || empty.Size != 0 || empty.SHA256 != fmt.Sprintf("%x", sha256.Sum256(nil)) {
		t.Fatalf("%+v %v", empty, err)
	}
	first, err := s.Save("https://example.test/", 1, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Save("https://example.test/", 1, []byte("first"))
	if err != nil || again.File != first.File {
		t.Fatalf("%+v %v", again, err)
	}
	changed, err := s.Save("https://example.test/", 1, []byte("changed"))
	if err != nil || changed.File == first.File {
		t.Fatalf("%+v %v", changed, err)
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("incomplete download") }

func TestFailedStreamPreservesCompleteFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saved, err := s.Save("https://example.test/app.js", 0, []byte("complete"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveReader("https://example.test/app.js", 0, io.MultiReader(strings.NewReader("partial"), failedReader{}))
	if err == nil {
		t.Fatal("accepted incomplete stream")
	}
	data, err := os.ReadFile(filepath.Join(dir, saved.File))
	if err != nil || string(data) != "complete" {
		t.Fatalf("%q %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(filepath.Join(dir, saved.File)))
	if err != nil || len(entries) != 1 {
		t.Fatalf("leftover files: %v %v", entries, err)
	}
}

func TestStableDistinctPaths(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	names := make(map[string]bool)
	for _, raw := range []string{"http://localhost:8080/a/app.js?v=1", "http://localhost:8080/b/app.js?v=1", "http://localhost:8080/a/app.js?v=2"} {
		saved, err := s.Save(raw, 0, []byte("first"))
		if err != nil {
			t.Fatal(err)
		}
		if names[saved.File] {
			t.Fatal("colliding paths")
		}
		names[saved.File] = true
		again, err := s.Save(raw, 0, []byte("updated"))
		if err != nil || saved.File != again.File {
			t.Fatalf("unstable path: %s %s %v", saved.File, again.File, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, saved.File))
		if err != nil || string(data) != "updated" {
			t.Fatalf("%q %v", data, err)
		}
	}
}

func TestVerifyCachedFileWithinRoot(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saved, err := s.Save("https://example.test/app.js", 0, []byte("content"))
	if err != nil || !s.Verify(saved, 7) || s.Verify(saved, 6) {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	outside := filepath.Join(dir, "outside.js")
	if err := os.WriteFile(outside, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../outside.js", outside, "missing.js", "."} {
		candidate := saved
		candidate.File = name
		if s.Verify(candidate, 7) {
			t.Errorf("accepted invalid cached file %q", name)
		}
	}
}
