package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAllowOriginValidation(t *testing.T) {
	for _, value := range []string{"*.example.test", "https://*.example.test", "https://example.test/path", "https://example.test?x=1", "https://user@example.test", "https://example.test#x", "file:///tmp", "http://localhost:0"} {
		_, err := New(Config{Timeout: time.Second, Workers: 1, MaxSize: 1024, AllowedOrigins: []string{value}})
		if err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestAllowedRedirectDoesNotForwardCustomHeaders(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"Authorization", "Cookie", "X-Api-Key", "Referer"} {
			if r.Header.Get(name) != "" {
				t.Errorf("forwarded %s", name)
			}
		}
		if r.UserAgent() != "linkz" {
			t.Errorf("user agent=%q", r.UserAgent())
		}
		fmt.Fprint(w, "cdn")
	}))
	defer cdn.Close()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "fixture" {
			t.Error("missing same-origin header")
		}
		http.Redirect(w, r, cdn.URL+"/app.js", http.StatusFound)
	}))
	defer page.Close()
	c, err := New(Config{Timeout: time.Second, Workers: 1, MaxSize: 1024, Redirects: true, AllowedOrigins: []string{cdn.URL}, Headers: []string{"Authorization: Bearer fixture", "Cookie: test=fixture", "X-Api-Key: fixture", "Referer: private", "User-Agent: private-client"}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	data, final, err := c.Get(context.Background(), page.URL, page.URL)
	if err != nil || string(data) != "cdn" || final != cdn.URL+"/app.js" {
		t.Fatalf("%q %q %v", data, final, err)
	}
}
