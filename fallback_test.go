package mux

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// The fallback chain is built once, not per request. A middleware that sets up
// state when it wraps must keep that state across 404s and 405s, and a
// constructor must not run again for every unmatched request.
func TestFallbackChainIsBuiltOnce(t *testing.T) {
	var built atomic.Int32
	counting := func(next http.Handler) http.Handler {
		built.Add(1)
		var seen atomic.Int32
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if seen.Add(1) > 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, req)
		})
	}

	r := New(WithNotFound(http.NotFoundHandler()))
	r.Use(counting)
	r.Get("/exists", func(http.ResponseWriter, *http.Request) {})

	got := make([]int, 0, 3)
	for range 3 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/missing", nil))
		got = append(got, w.Code)
	}
	// The wrapper's own counter survives, so only the first request passes.
	if fmt.Sprint(got) != fmt.Sprint([]int{404, 429, 429}) {
		t.Errorf("codes = %v, want [404 429 429]: the chain was rebuilt", got)
	}

	// One wrapping for the route, one for each of the two fallback chains,
	// and nothing more however many requests arrive.
	after := built.Load()
	for range 20 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/missing", nil))
	}
	if built.Load() != after {
		t.Errorf("constructions grew from %d to %d while serving", after, built.Load())
	}
	if after > 3 {
		t.Errorf("constructions = %d, want at most one per route and per chain", after)
	}
}

// Concurrent unmatched requests must not each build a chain of their own.
func TestFallbackChainUnderConcurrency(t *testing.T) {
	var built atomic.Int32
	r := New(WithNotFound(http.NotFoundHandler()), WithMethodNotAllowed(http.NotFoundHandler()))
	r.Use(func(next http.Handler) http.Handler {
		built.Add(1)
		return next
	})
	r.Get("/exists", func(http.ResponseWriter, *http.Request) {})

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/missing", nil)
			if i%2 == 0 {
				req = httptest.NewRequest(http.MethodPost, "/exists", nil)
			}
			r.ServeHTTP(httptest.NewRecorder(), req)
		}(i)
	}
	wg.Wait()

	if built.Load() > 3 {
		t.Errorf("constructions = %d, want at most one per route and per chain", built.Load())
	}
}

// The Allow header belongs to the request it was computed for, so a shared
// chain must not hand one request's Allow to another.
func TestAllowIsPerRequest(t *testing.T) {
	r := New(WithMethodNotAllowed(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
		})))
	r.Get("/only-get", func(http.ResponseWriter, *http.Request) {})
	r.Post("/only-post", func(http.ResponseWriter, *http.Request) {})

	// The values are whatever the standard mux computes; what this checks is
	// that each request gets its own, and that the shared chain does not hand
	// one request the Allow of the one before it.
	cases := []struct{ method, path, want string }{
		{http.MethodPost, "/only-get", "GET, HEAD"},
		{http.MethodGet, "/only-post", "POST"},
		{http.MethodPost, "/only-get", "GET, HEAD"},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		if got := w.Header().Get("Allow"); got != c.want {
			t.Errorf("%s %s: Allow = %q, want %q", c.method, c.path, got, c.want)
		}
	}
}

// A custom fallback must report this router's match state, not the one the
// request arrived with from an outer mux.
func TestCustomFallbackClearsOuterMatch(t *testing.T) {
	seen := struct{ pattern, tenant string }{}
	record := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seen.pattern = req.Pattern
		seen.tenant = req.PathValue("tenant")
		w.WriteHeader(http.StatusNotFound)
	})

	child := New(WithNotFound(record))
	child.Get("/exists", func(http.ResponseWriter, *http.Request) {})

	parent := http.NewServeMux()
	parent.Handle("/tenant/{tenant}/{rest...}", child)

	w := httptest.NewRecorder()
	parent.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tenant/acme/missing", nil))

	if seen.pattern != "" || seen.tenant != "" {
		t.Errorf("fallback saw pattern=%q tenant=%q, want both empty",
			seen.pattern, seen.tenant)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// Middleware on the fallback path sees the same cleared state as the handler.
func TestFallbackMiddlewareSeesClearedMatch(t *testing.T) {
	var pattern string
	child := New(WithNotFound(http.NotFoundHandler()))
	child.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			pattern = req.Pattern
			next.ServeHTTP(w, req)
		})
	})
	child.Get("/exists", func(http.ResponseWriter, *http.Request) {})

	parent := http.NewServeMux()
	parent.Handle("/tenant/{tenant}/{rest...}", child)
	parent.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/tenant/acme/missing", nil))

	if pattern != "" {
		t.Errorf("middleware saw Pattern = %q, want empty", pattern)
	}
}
