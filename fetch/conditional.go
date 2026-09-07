package fetcher

import (
	"context"
	"net/http"
	"strings"
)

type Validators struct{ ETag, LastModified string }

func (v Validators) Valid() bool {
	return v.ETag != "" && validETag(v.ETag) || v.ETag == "" && validModified(v.LastModified)
}

func validETag(value string) bool {
	value = strings.TrimPrefix(value, "W/")
	if len(value) < 2 || len(value) > 4096 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	for _, c := range []byte(value[1 : len(value)-1]) {
		if c < 0x21 || c == '"' || c == 0x7f {
			return false
		}
	}
	return true
}

func validModified(value string) bool {
	_, err := http.ParseTime(value)
	return value != "" && err == nil
}

func (c *Client) CanRevalidate(raw, origin string) bool {
	u, err := ParseURL(raw)
	if err != nil {
		return false
	}
	scope, err := c.Scope(origin)
	if err != nil {
		return false
	}
	return scope.Allows(u) && (!SameOrigin(scope.origin, u) || !c.customHeaders)
}

func (c *Client) OpenConditional(ctx context.Context, raw, origin string, v Validators) (*Response, error) {
	return c.open(ctx, raw, origin, v)
}

// This is a deliberately conservative private revalidation cache. Requests with
// custom headers are excluded, as are unsupported Vary dimensions and redirects.
func (c *Client) reusable(resp *http.Response, raw, origin string) bool {
	if !c.CanRevalidate(raw, origin) || resp.Request.URL.String() != raw || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified) {
		return false
	}
	for _, value := range resp.Header.Values("Cache-Control") {
		for _, directive := range strings.Split(value, ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(directive), "=")
			if strings.EqualFold(name, "no-store") {
				return false
			}
		}
	}
	for _, value := range resp.Header.Values("Vary") {
		for _, name := range strings.Split(value, ",") {
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "", "accept-encoding", "user-agent":
			default:
				return false
			}
		}
	}
	return true
}

func responseValidators(h http.Header) Validators {
	v := Validators{}
	if value := h.Get("ETag"); validETag(value) {
		v.ETag = value
	}
	if value := h.Get("Last-Modified"); validModified(value) {
		v.LastModified = value
	}
	return v
}
