package fileutils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStableDistinctPaths(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	names := make(map[string]bool)
	for _, raw := range []string{"http://localhost:8080/a/app.js?v=1", "http://localhost:8080/b/app.js?v=1", "http://localhost:8080/a/app.js?v=2"} {
		name, err := s.Save(raw, 0, []byte("first"))
		if err != nil {
			t.Fatal(err)
		}
		if names[name] {
			t.Fatal("colliding paths")
		}
		names[name] = true
		again, err := s.Save(raw, 0, []byte("updated"))
		if err != nil || name != again {
			t.Fatalf("unstable path: %s %s %v", name, again, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != "updated" {
			t.Fatalf("%q %v", data, err)
		}
	}
}
