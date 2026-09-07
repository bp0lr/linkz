// Package fetcher performs bounded, origin-scoped HTTP requests.
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http/httpguts"
)

var ErrTooLarge = errors.New("response exceeds max-size")

type Config struct {
	Timeout        time.Duration
	Proxy          string
	Headers        []string
	Redirects      bool
	MaxSize        int64
	Workers        int
	AllowedOrigins []string
}

type Client struct {
	http          *http.Client
	headers       http.Header
	maxSize       int64
	slots         chan struct{}
	allowed       map[string]bool
	customHeaders bool
}

type Response struct {
	Body       io.ReadCloser
	URL        string
	Status     int
	Validators Validators
	Cacheable  bool
}

type scopeKey struct{}

func New(conf Config) (*Client, error) {
	if conf.Timeout <= 0 || conf.MaxSize <= 0 || conf.MaxSize > 1<<30 || conf.Workers < 1 {
		return nil, errors.New("invalid HTTP limits")
	}
	allowed, err := parseOrigins(conf.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = conf.Workers * 2
	transport.MaxIdleConnsPerHost = conf.Workers
	transport.MaxConnsPerHost = conf.Workers
	transport.IdleConnTimeout = 90 * time.Second
	if conf.Proxy != "" {
		proxy, err := url.Parse(conf.Proxy)
		if err != nil || proxy.Hostname() == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") {
			return nil, errors.New("proxy must be a valid HTTP or HTTPS URL")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	c := &Client{http: &http.Client{Transport: transport, Timeout: conf.Timeout}, headers: make(http.Header), maxSize: conf.MaxSize, slots: make(chan struct{}, conf.Workers)}
	c.allowed = allowed
	c.customHeaders = len(conf.Headers) != 0
	c.headers.Set("User-Agent", "linkz")
	for _, header := range conf.Headers {
		name, value, ok := strings.Cut(header, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			return nil, errors.New("header must have a valid Name: value format")
		}
		if strings.EqualFold(name, "Host") {
			return nil, errors.New("overriding Host is not supported")
		}
		c.headers.Set(name, value)
	}
	c.http.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !conf.Redirects {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		scope, ok := req.Context().Value(scopeKey{}).(*Scope)
		if !ok || !scope.Allows(req.URL) {
			return errors.New("redirect leaves allowed origins")
		}
		req.Header = c.requestHeaders(req.URL, scope)
		return nil
	}
	return c, nil
}

func ParseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("expected an absolute HTTP or HTTPS URL without credentials")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("URL port must be between 1 and 65535")
		}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u, nil
}

func SameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func port(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (c *Client) Open(ctx context.Context, raw, origin string) (*Response, error) {
	return c.open(ctx, raw, origin, Validators{})
}

func (c *Client) open(ctx context.Context, raw, origin string, validators Validators) (*Response, error) {
	u, err := ParseURL(raw)
	if err != nil {
		return nil, err
	}
	scope, err := c.Scope(origin)
	if err != nil {
		return nil, err
	}
	if !scope.Allows(u) {
		return nil, errors.New("resource leaves allowed origins")
	}
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	handedOff := false
	defer func() {
		if !handedOff {
			<-c.slots
		}
	}()
	req, err := http.NewRequestWithContext(context.WithValue(ctx, scopeKey{}, scope), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.requestHeaders(u, scope)
	conditional := validators.ETag != "" || validators.LastModified != ""
	if conditional {
		if !validators.Valid() || !c.CanRevalidate(u.String(), origin) {
			return nil, errors.New("request is not eligible for conditional reuse")
		}
		if validators.ETag != "" {
			req.Header.Set("If-None-Match", validators.ETag)
		} else {
			req.Header.Set("If-Modified-Since", validators.LastModified)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	notModified := resp.StatusCode == http.StatusNotModified && conditional && resp.Request.URL.String() == u.String() && (resp.Request.Header.Get("If-None-Match") != "" || resp.Request.Header.Get("If-Modified-Since") != "")
	if !notModified && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	if !notModified && resp.ContentLength > c.maxSize {
		resp.Body.Close()
		return nil, ErrTooLarge
	}
	body := struct {
		io.Reader
		io.Closer
	}{&limitedReader{r: resp.Body, remaining: c.maxSize}, resp.Body}
	handedOff = true
	return &Response{Body: &leasedBody{ReadCloser: body, release: func() { <-c.slots }}, URL: resp.Request.URL.String(), Status: resp.StatusCode, Validators: responseValidators(resp.Header), Cacheable: c.reusable(resp, u.String(), origin)}, nil
}

// Keep the concurrency slot until the response body is closed, including while
// the caller streams it to disk. All hosts and both work queues share this cap.
type leasedBody struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

func (b *leasedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}

func (c *Client) Get(ctx context.Context, raw, origin string) ([]byte, string, error) {
	resp, err := c.Open(ctx, raw, origin)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.URL, err
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

type limitedReader struct {
	r         io.Reader
	remaining int64
}

func (r *limitedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.r.Read(extra[:])
		if n > 0 {
			return 0, ErrTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	return n, err
}
