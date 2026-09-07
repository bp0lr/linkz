package main

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"

	web "github.com/bp0lr/linkz/fetch"
	filter "github.com/bp0lr/linkz/static"
	"golang.org/x/net/html"
)

// Preserve discovery of quoted JS paths while parsing actual script tags as HTML.
var quotedReference = regexp.MustCompile(`["']([^"'<>\s]+)["']`)

type inlineScript struct {
	index int
	code  []byte
}
type pageScripts struct {
	links  []string
	inline []inlineScript
}

func extract(page string, source []byte, includeLibs bool) (pageScripts, error) {
	origin, err := web.ParseURL(page)
	if err != nil {
		return pageScripts{}, err
	}
	base := origin
	var refs []string
	var result pageScripts
	baseSeen, inScript, saveScript := false, false, false
	scriptIndex := 0
	tokenizer := html.NewTokenizer(bytes.NewReader(source))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if tokenizer.Err() != io.EOF {
				return result, fmt.Errorf("parse HTML: %w", tokenizer.Err())
			}
			goto resolved
		case html.StartTagToken, html.SelfClosingTagToken:
			t := tokenizer.Token()
			attrs := make(map[string]string, len(t.Attr))
			for _, a := range t.Attr {
				if _, exists := attrs[a.Key]; !exists {
					attrs[a.Key] = a.Val
				}
			}
			if t.Data == "base" && !baseSeen {
				if href, exists := attrs["href"]; exists {
					baseSeen = true
					if ref, err := url.Parse(strings.TrimSpace(href)); err == nil {
						base = origin.ResolveReference(ref)
					}
				}
			}
			if t.Data == "script" {
				scriptIndex++
				inScript = true
				src, external := attrs["src"]
				saveScript = !external && isJavaScript(attrs["type"])
				if external && strings.TrimSpace(src) != "" && isJavaScript(attrs["type"]) {
					refs = append(refs, src)
				}
			}
		case html.TextToken:
			if inScript && saveScript {
				code := append([]byte(nil), tokenizer.Text()...)
				if len(bytes.TrimSpace(code)) > 0 {
					result.inline = append(result.inline, inlineScript{scriptIndex, code})
				}
			}
		case html.EndTagToken:
			if tokenizer.Token().Data == "script" {
				inScript, saveScript = false, false
			}
		}
	}
resolved:
	for _, match := range quotedReference.FindAllSubmatch(source, -1) {
		ref := stdhtml.UnescapeString(string(match[1]))
		if u, err := url.Parse(ref); err == nil {
			ext := strings.ToLower(path.Ext(u.Path))
			if ext == ".js" || ext == ".mjs" {
				refs = append(refs, ref)
			}
		}
	}
	seen := make(map[string]bool)
	for _, raw := range refs {
		ref, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		u, err := web.ParseURL(base.ResolveReference(ref).String())
		if err != nil || !web.SameOrigin(origin, u) || (!includeLibs && filter.Exist(path.Base(u.Path))) {
			continue
		}
		if !seen[u.String()] {
			result.links = append(result.links, u.String())
			seen[u.String()] = true
		}
	}
	return result, nil
}

func isJavaScript(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(strings.SplitN(kind, ";", 2)[0]))
	switch kind {
	case "", "module", "text/javascript", "application/javascript", "text/ecmascript", "application/ecmascript":
		return true
	default:
		return false
	}
}
