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

type Saved struct {
	File   string
	Size   int64
	SHA256 string
}

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
// of zero denotes an external resource. Inline identities also include content.
func (s *Store) Save(raw string, inlineIndex int, content []byte) (Saved, error) {
	return s.SaveReader(raw, inlineIndex, bytes.NewReader(content))
}

// SaveReader streams the source into a temporary file. A failed read leaves any
// previous complete file in place and removes the temporary file.
func (s *Store) SaveReader(raw string, inlineIndex int, content io.Reader) (Saved, error) {
	u, err := web.ParseURL(raw)
	if err != nil {
		return Saved{}, err
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
	if err := s.root.MkdirAll(dir, 0750); err != nil {
		return Saved{}, err
	}
	tmp := filepath.Join(dir, ".linkz-"+rand.Text()+".tmp")
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return Saved{}, err
	}
	defer s.root.Remove(tmp)
	hash := sha256.New()
	size, writeErr := io.Copy(io.MultiWriter(f, hash), content)
	closeErr := f.Close()
	if writeErr != nil {
		return Saved{}, writeErr
	}
	if closeErr != nil {
		return Saved{}, closeErr
	}
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	if inlineIndex > 0 {
		key += ":" + digest
	}
	name := filepath.Join(dir, fmt.Sprintf("%s-%x.js", prefix, sha256.Sum256([]byte(key))))
	if err := s.root.Rename(tmp, name); err != nil {
		return Saved{}, err
	}
	return Saved{File: filepath.ToSlash(name), Size: size, SHA256: digest}, nil
}
