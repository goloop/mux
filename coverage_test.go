package mux

import (
	"fmt"
	"net/http"
	"testing"
)

// TestAllMethodHelpers exercises every verb helper and its error-returning
// counterpart so the whole surface is covered.
func TestAllMethodHelpers(t *testing.T) {
	ok := func(w http.ResponseWriter, req *http.Request) { fmt.Fprint(w, "ok") }

	type reg struct {
		method string
		plain  func(*Router, string, http.HandlerFunc)
	}
	regs := []reg{
		{http.MethodGet, (*Router).Get},
		{http.MethodPost, (*Router).Post},
		{http.MethodPut, (*Router).Put},
		{http.MethodPatch, (*Router).Patch},
		{http.MethodDelete, (*Router).Delete},
		{http.MethodOptions, (*Router).Options},
		{http.MethodHead, (*Router).Head},
	}
	for _, rg := range regs {
		r := New()
		rg.plain(r, "/x", ok)
		rec := serve(r, rg.method, "/x")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", rg.method, rec.Code)
		}
	}
}

func TestAllErrorHelpers(t *testing.T) {
	boom := func(w http.ResponseWriter, req *http.Request) error {
		return fmt.Errorf("boom")
	}
	eh := WithErrorHandler(func(w http.ResponseWriter, req *http.Request, err error) {
		w.WriteHeader(http.StatusBadGateway)
	})

	type reg struct {
		method string
		e      func(*Router, string, HandlerFunc)
	}
	regs := []reg{
		{http.MethodGet, (*Router).GetE},
		{http.MethodPost, (*Router).PostE},
		{http.MethodPut, (*Router).PutE},
		{http.MethodPatch, (*Router).PatchE},
		{http.MethodDelete, (*Router).DeleteE},
	}
	for _, rg := range regs {
		r := New(eh)
		rg.e(r, "/x", boom)
		rec := serve(r, rg.method, "/x")
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("%sE: status = %d, want 502", rg.method, rec.Code)
		}
	}

	// MethodE and HandleError round out the error surface.
	r := New(eh)
	r.MethodE(http.MethodGet, "/m", boom)
	if rec := serve(r, http.MethodGet, "/m"); rec.Code != http.StatusBadGateway {
		t.Fatalf("MethodE: status = %d, want 502", rec.Code)
	}
	r2 := New(eh)
	r2.HandleError("GET /h", boom)
	if rec := serve(r2, http.MethodGet, "/h"); rec.Code != http.StatusBadGateway {
		t.Fatalf("HandleError: status = %d, want 502", rec.Code)
	}
}

func TestHandleFunc(t *testing.T) {
	r := New()
	r.HandleFunc("GET /hf", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, "hf")
	})
	if rec := serve(r, http.MethodGet, "/hf"); rec.Body.String() != "hf" {
		t.Fatalf("body = %q, want hf", rec.Body.String())
	}
}

func TestMountAtRoot(t *testing.T) {
	echo := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, req.URL.Path)
	})
	r := New()
	r.Mount("/", echo)
	if rec := serve(r, http.MethodGet, "/anything"); rec.Body.String() != "/anything" {
		t.Fatalf("root mount: body = %q, want /anything", rec.Body.String())
	}
}

// TestNotFoundConfiguredButMethodMismatch covers the branch where only a custom
// not-found handler is set and the request is a method mismatch (default 405).
func TestNotFoundConfiguredButMethodMismatch(t *testing.T) {
	r := New(WithNotFound(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusGone)
		})))
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {})

	rec := serve(r, http.MethodPost, "/x")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want default 405 (no custom 405 handler)", rec.Code)
	}
}

func TestJoinPatternHost(t *testing.T) {
	got := joinPattern("/api", "example.com/users")
	if got != "example.com/api/users" {
		t.Fatalf("joinPattern host = %q, want example.com/api/users", got)
	}
	// No path portion at all: the whole rest is treated as a path fragment.
	if got := joinPattern("/api", "bare"); got != "/api/bare" {
		t.Fatalf("joinPattern no-slash = %q, want /api/bare", got)
	}
}

func TestJoinPathEmpty(t *testing.T) {
	if got := joinPath("/api", ""); got != "/api/" {
		t.Fatalf("joinPath empty path = %q, want /api/", got)
	}
}

// TestMethodNotAllowedConfiguredButNotFound covers the branch where a custom
// 405 handler is set and a genuine 404 falls through to the default not-found.
func TestMethodNotAllowedConfiguredButNotFound(t *testing.T) {
	r := New(WithMethodNotAllowed(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})))
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {})

	rec := serve(r, http.MethodGet, "/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want default 404", rec.Code)
	}
}

func TestSplitPattern(t *testing.T) {
	m, rest := splitPattern("GET example.com/x")
	if m != "GET" || rest != "example.com/x" {
		t.Fatalf("splitPattern = %q,%q", m, rest)
	}
	if m, rest := splitPattern("/x"); m != "" || rest != "/x" {
		t.Fatalf("splitPattern no-method = %q,%q", m, rest)
	}
}
