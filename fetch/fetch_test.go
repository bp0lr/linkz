package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestConnectionReuse(t *testing.T) {
	var connections atomic.Int64
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	s.Start()
	defer s.Close()
	c := testClient(t, 1024, false)
	for i := 0; i < 4; i++ {
		if _, _, err := c.Get(context.Background(), s.URL, s.URL); err != nil {
			t.Fatal(err)
		}
	}
	if connections.Load() != 1 {
		t.Fatalf("connections=%d", connections.Load())
	}
}

func testClient(t *testing.T, max int64, redirects bool) *Client {
	t.Helper()
	c, err := New(Config{Timeout: time.Second, MaxSize: max, Workers: 2, Redirects: redirects})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestResponseLimits(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if streaming {
				w.(http.Flusher).Flush()
			}
			fmt.Fprint(w, "12345")
		}))
		c := testClient(t, 4, false)
		_, _, err := c.Get(context.Background(), s.URL, s.URL)
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("streaming=%v err=%v", streaming, err)
		}
		c = testClient(t, 5, false)
		data, _, err := c.Get(context.Background(), s.URL, s.URL)
		if err != nil || string(data) != "12345" {
			t.Errorf("boundary: %q %v", data, err)
		}
		s.Close()
	}
}

func TestRejectHTTPErrorAndTLSFailure(t *testing.T) {
	c := testClient(t, 1024, false)
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	if _, _, err := c.Get(context.Background(), s.URL, s.URL); err == nil {
		t.Fatal("accepted 404")
	}
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer tls.Close()
	if _, _, err := c.Get(context.Background(), tls.URL, tls.URL); err == nil {
		t.Fatal("accepted untrusted certificate")
	}
}

func TestRedirectOriginPolicy(t *testing.T) {
	c := testClient(t, 1024, true)
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("contacted a different origin") }))
	defer destination.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer s.Close()
	if _, _, err := c.Get(context.Background(), s.URL, s.URL); err == nil {
		t.Fatal("accepted a redirect to another origin")
	}
}

func TestCancellation(t *testing.T) {
	c := testClient(t, 1024, false)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, _, err := c.Get(ctx, s.URL, s.URL); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}
