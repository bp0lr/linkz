package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	web "github.com/bp0lr/linkz/fetch"
)

const manifestVersion = 2
const maxManifestSize = 256 << 20

type artifactKey struct {
	Source, Kind, URL string
	Index             int
}
type pageState struct{ complete, failed bool }
type inventory struct {
	artifacts map[artifactKey]artifact
	pages     map[string]pageState
}

func sourceKey(a artifact) string {
	if u, err := web.ParseURL(a.Source); err == nil {
		return u.String()
	}
	if u, err := web.ParseURL(a.Page); err == nil {
		return u.String()
	}
	return a.Source
}

func keyFor(a artifact) artifactKey {
	k := artifactKey{Source: sourceKey(a), Kind: a.Kind}
	if a.Kind == "inline" {
		k.Index = a.InlineIndex
	} else {
		k.URL = a.URL
	}
	return k
}

func loadInventory(name string) (*inventory, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxManifestSize {
		return nil, fmt.Errorf("manifest must be a regular file of at most 256 MiB")
	}
	scanner := bufio.NewScanner(io.LimitReader(f, maxManifestSize+1))
	scanner.Buffer(make([]byte, 4096), 8<<20)
	inv := &inventory{artifacts: make(map[artifactKey]artifact), pages: make(map[string]pageState)}
	line, total := 0, 0
	for scanner.Scan() {
		line++
		total += len(scanner.Bytes()) + 1
		if total > maxManifestSize {
			return nil, fmt.Errorf("manifest exceeds 256 MiB")
		}
		data := strings.TrimSpace(scanner.Text())
		if line == 1 {
			data = strings.TrimPrefix(data, "\ufeff")
		}
		if data == "" {
			continue
		}
		var a artifact
		if err := json.Unmarshal([]byte(data), &a); err != nil {
			return nil, fmt.Errorf("manifest line %d: %w", line, err)
		}
		if err := validateRecord(a); err != nil {
			return nil, fmt.Errorf("manifest line %d: %w", line, err)
		}
		a.SHA256 = strings.ToLower(a.SHA256)
		key := sourceKey(a)
		page := inv.pages[key]
		switch a.Kind {
		case "page":
			page.complete = true
		case "page_error":
			page.failed = true
		default:
			k := keyFor(a)
			if previous, exists := inv.artifacts[k]; exists && previous != a {
				return nil, fmt.Errorf("manifest line %d: conflicting duplicate resource", line)
			}
			inv.artifacts[k] = a
		}
		inv.pages[key] = page
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return inv, nil
}

func validateRecord(a artifact) error {
	if (a.SchemaVersion != 1 && a.SchemaVersion != manifestVersion) || a.Source == "" {
		return fmt.Errorf("unsupported schema version or missing source")
	}
	if a.Size < 0 {
		return fmt.Errorf("negative size_bytes")
	}
	if a.SHA256 != "" {
		if hash, err := hex.DecodeString(a.SHA256); err != nil || len(hash) != 32 {
			return fmt.Errorf("invalid SHA-256")
		}
	}
	switch a.Kind {
	case "page":
		if a.SchemaVersion < 2 || a.Status != "processed" {
			return fmt.Errorf("invalid page completion record")
		}
	case "page_error":
		if a.Status != "error" || a.Error == "" {
			return fmt.Errorf("invalid page error record")
		}
	case "external", "inline":
		if _, err := web.ParseURL(a.URL); err != nil {
			return fmt.Errorf("invalid resource URL")
		}
		if a.Kind == "inline" && a.InlineIndex < 1 || a.Kind == "external" && a.InlineIndex != 0 {
			return fmt.Errorf("invalid inline_index")
		}
		if a.Status != "listed" && a.Status != "saved" && a.Status != "error" {
			return fmt.Errorf("invalid resource status")
		}
		if a.Status == "saved" && (a.File == "" || a.SHA256 == "") {
			return fmt.Errorf("saved resource lacks file or hash")
		}
		if a.Status == "error" && a.Error == "" {
			return fmt.Errorf("error record lacks error description")
		}
	default:
		return fmt.Errorf("unsupported record kind %q", a.Kind)
	}
	return nil
}
