package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidatorsAndUnexpected304(t *testing.T) {
	for _, tag := range []string{"*", "bare", "\"line\n\"", "\"a\", \"b\""} {
		if (Validators{ETag: tag}).Valid() {
			t.Errorf("accepted %q", tag)
		}
	}
	for _, tag := range []string{`"v1"`, `W/"v1"`, `""`} {
		if !(Validators{ETag: tag}).Valid() {
			t.Errorf("rejected %q", tag)
		}
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(304) }))
	defer s.Close()
	c := testClient(t, 1024, false)
	if _, _, err := c.Get(context.Background(), s.URL, s.URL); err == nil {
		t.Fatal("accepted an unconditional 304")
	}
}

func TestRedirectDropsValidators(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/final.js", 302)
			return
		}
		if r.Header.Get("If-None-Match") != "" {
			t.Error("forwarded a validator to a different resource")
		}
		w.Header().Set("ETag", `"new"`)
		fmt.Fprint(w, "new")
	}))
	defer s.Close()
	c := testClient(t, 1024, true)
	resp, err := c.OpenConditional(context.Background(), s.URL, s.URL, Validators{ETag: `"old"`})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Status != 200 || resp.Cacheable {
		t.Fatalf("response=%+v", resp)
	}
}
