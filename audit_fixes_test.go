package mux

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCustomNotFoundKeepsPathValue guards BUG-01: configuring a custom 404 (or
// 405) handler must not break PathValue/Param on matched routes.
func TestCustomNotFoundKeepsPathValue(t *testing.T) {
	r := New(mux404())
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "id=%s pattern=%s", req.PathValue("id"), req.Pattern)
	})

	rec := serve(r, http.MethodGet, "/users/42")
	if got, want := rec.Body.String(), "id=42 pattern=GET /users/{id}"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestMountStripExactPrefixRedirects guards BUG-02: a request for exactly the
// mount prefix redirects to the subtree root, not the site root.
func TestMountStripExactPrefixRedirects(t *testing.T) {
	sub := http.NewServeMux()
	sub.HandleFunc("GET /{$}", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, "panel-root")
	})
	r := New()
	r.MountStrip("/panel", sub)

	rec := serve(r, http.MethodGet, "/panel")
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("GET /panel status = %d, want 307", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/panel/" {
		t.Errorf("Location = %q, want %q", loc, "/panel/")
	}

	rec = serve(r, http.MethodGet, "/panel/")
	if rec.Code != http.StatusOK || rec.Body.String() != "panel-root" {
		t.Errorf("GET /panel/ = %d %q, want 200 panel-root", rec.Code, rec.Body.String())
	}
}

// TestCustomNotFoundOptionsAsterisk guards BUG-03: "OPTIONS *" still gets the
// standard 400, not the custom 404.
func TestCustomNotFoundOptionsAsterisk(t *testing.T) {
	r := New(mux404())
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {})

	req := httptest.NewRequest(http.MethodOptions, "*", nil)
	req.RequestURI = "*"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("OPTIONS * status = %d, want 400", rec.Code)
	}
}

// TestCustomNotFoundReplaysRedirect guards BUG-04: a dirty path that cleans to
// an unmatched route still gets the standard redirect, not the custom 404.
func TestCustomNotFoundReplaysRedirect(t *testing.T) {
	r := New(mux404())
	r.Get("/foo/bar", func(w http.ResponseWriter, req *http.Request) {})

	rec := serve(r, http.MethodGet, "/foo//baz")
	if rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("dirty path status = %d, want a redirect (301/307), not custom 404", rec.Code)
	}
	if rec.Code == 499 {
		t.Error("dirty path returned the custom 404 instead of the standard redirect")
	}
}

// mux404 returns an option installing a recognizable custom 404 (status 499).
func mux404() Option {
	return WithNotFound(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(499)
		fmt.Fprint(w, "custom-404")
	}))
}
