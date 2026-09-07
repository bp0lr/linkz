package main

import (
	web "github.com/bp0lr/linkz/fetch"
	files "github.com/bp0lr/linkz/fileutils"
)

func savedArtifact(a artifact) files.Saved {
	return files.Saved{File: a.File, Size: a.Size, SHA256: a.SHA256}
}

func cacheIndex(inv *inventory) map[string]artifact {
	cache := make(map[string]artifact)
	conflicts := make(map[string]bool)
	for _, a := range inv.artifacts {
		if a.Kind != "external" || a.Status != "saved" || !a.Cacheable || !(web.Validators{ETag: a.ETag, LastModified: a.LastModified}).Valid() {
			continue
		}
		u, err := web.ParseURL(a.URL)
		if err != nil {
			continue
		}
		if a.FinalURL != u.String() {
			continue
		}
		origin, err := web.ParseURL(a.Origin)
		if err != nil {
			continue
		}
		a.Origin = web.Origin(origin)
		key := a.Origin + "\x00" + u.String()
		if conflicts[key] {
			continue
		}
		if previous, exists := cache[key]; exists && (savedArtifact(previous) != savedArtifact(a) || previous.ETag != a.ETag || previous.LastModified != a.LastModified) {
			delete(cache, key)
			conflicts[key] = true
			continue
		}
		cache[key] = a
	}
	return cache
}
