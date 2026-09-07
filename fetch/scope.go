package fetcher

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Scope combines the input page origin with an immutable explicit allowlist.
type Scope struct {
	origin  *url.URL
	allowed map[string]bool
}

func Origin(u *url.URL) string {
	return strings.ToLower(u.Scheme) + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port(u))
}

func parseOrigins(values []string) (map[string]bool, error) {
	allowed := make(map[string]bool, len(values))
	for _, value := range values {
		u, err := url.Parse(strings.TrimSpace(value))
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(u.Host, "*") {
			return nil, errors.New("allow-origin requires an exact HTTP or HTTPS origin without paths, credentials, queries or wildcards")
		}
		if _, err := ParseURL(value); err != nil {
			return nil, err
		}
		allowed[Origin(u)] = true
	}
	return allowed, nil
}

func (c *Client) Scope(origin string) (*Scope, error) {
	u, err := ParseURL(origin)
	if err != nil {
		return nil, err
	}
	return &Scope{origin: u, allowed: c.allowed}, nil
}

func (s *Scope) Allows(u *url.URL) bool {
	return u.User == nil && (u.Scheme == "http" || u.Scheme == "https") && (SameOrigin(s.origin, u) || s.allowed[Origin(u)])
}

func (c *Client) requestHeaders(target *url.URL, scope *Scope) http.Header {
	if SameOrigin(scope.origin, target) {
		return c.headers.Clone()
	}
	// Do not guess which custom headers contain credentials. None leave the input
	// page origin, even when another origin has explicitly been allowed.
	return http.Header{"User-Agent": {"linkz"}}
}
