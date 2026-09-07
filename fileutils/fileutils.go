// Package fileutils stores scripts within an explicitly selected output root.
package fileutils

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	web "github.com/bp0lr/linkz/fetch"
)

type Store struct{ root *os.Root }

func NewStore(folder string) (*Store, error) {
	if err := os.MkdirAll(folder, 0750); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return nil, err
	}
	return &Store{root}, nil
}

func (s *Store) Close() error { return s.root.Close() }

// Save uses the full URL as identity, including query parameters. An inline index
// of zero denotes an external resource. Successful reruns replace the same file.
func (s *Store) Save(raw string, inlineIndex int, content []byte) (string, error) {
	return s.SaveReader(raw, inlineIndex, bytes.NewReader(content))
}

// SaveReader streams the source into a temporary file. A failed read leaves any
// previous complete file in place and removes the temporary file.
func (s *Store) SaveReader(raw string, inlineIndex int, content io.Reader) (string, error) {
	u, err := web.ParseURL(raw)
	if err != nil {
		return "", err
	}
	host := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, u.Host)
	if len(host) > 80 {
		host = host[:80]
	}
	dir := "host_" + host
	key, prefix := "url:"+u.String(), "script"
	if inlineIndex > 0 {
		key, prefix = fmt.Sprintf("inline:%s:%d", u.String(), inlineIndex), "inline"
	}
	name := filepath.Join(dir, fmt.Sprintf("%s-%x.js", prefix, sha256.Sum256([]byte(key))))
	if err := s.root.MkdirAll(dir, 0750); err != nil {
		return "", err
	}
	tmp := filepath.Join(dir, ".linkz-"+rand.Text()+".tmp")
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer s.root.Remove(tmp)
	_, writeErr := io.Copy(f, content)
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := s.root.Rename(tmp, name); err != nil {
		return "", err
	}
	return filepath.ToSlash(name), nil
}
