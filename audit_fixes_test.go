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

// A zero-value Router (not built with New) fails with a clear panic rather than
// a raw nil dereference.
func TestZeroValueRouterPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("zero-value Router ServeHTTP did not panic")
		}
		if msg, ok := r.(string); !ok || !contains(msg, "mux.New()") {
			t.Fatalf("panic = %v, want a mux.New() hint", r)
		}
	}()
	var r Router
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

// Root middleware now covers custom 404 and 405 responses.
func TestNotFoundRunsThroughMiddleware(t *testing.T) {
	r := New(mux404())
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-MW", "1")
			next.ServeHTTP(w, req)
		})
	})
	r.Get("/exists", func(http.ResponseWriter, *http.Request) {})

	rec := serve(r, http.MethodGet, "/missing")
	if rec.Code != 499 {
		t.Fatalf("status = %d, want custom 404 (499)", rec.Code)
	}
	if rec.Header().Get("X-MW") != "1" {
		t.Error("middleware did not run for the custom 404 response")
	}
}

// The standard 405 fallback (only a 404 configured) also runs through
// middleware and keeps the Allow header.
func TestMethodNotAllowedThroughMiddleware(t *testing.T) {
	r := New(mux404())
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-MW", "1")
			next.ServeHTTP(w, req)
		})
	})
	r.Get("/only-get", func(http.ResponseWriter, *http.Request) {})

	rec := serve(r, http.MethodPost, "/only-get")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if rec.Header().Get("X-MW") != "1" {
		t.Error("middleware did not run for the 405 response")
	}
	if rec.Header().Get("Allow") == "" {
		t.Error("Allow header lost on the 405 response")
	}
}

// A handler that writes and then returns an error must not have the error
// handler write a second (corrupting) response.
func TestErrorHandlerSkippedAfterWrite(t *testing.T) {
	r := New()
	r.GetE("/x", func(w http.ResponseWriter, req *http.Request) error {
		w.WriteHeader(http.StatusTeapot)
		fmt.Fprint(w, "partial")
		return fmt.Errorf("too late")
	})
	rec := serve(r, http.MethodGet, "/x")
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 (error handler must not overwrite)", rec.Code)
	}
	if rec.Body.String() != "partial" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "partial")
	}
}

// A handler that returns an error before writing still reaches the error
// handler.
func TestErrorHandlerRunsWhenNothingWritten(t *testing.T) {
	r := New()
	r.GetE("/x", func(w http.ResponseWriter, req *http.Request) error {
		return fmt.Errorf("boom")
	})
	rec := serve(r, http.MethodGet, "/x")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 from the error handler", rec.Code)
	}
}

// A slashless token inside a Route prefix is a relative path fragment, not a
// host.
func TestJoinPatternSlashlessIsPathFragment(t *testing.T) {
	if got := joinPattern("/api", "users"); got != "/api/users" {
		t.Fatalf("joinPattern = %q, want /api/users", got)
	}
	if got := joinPattern("/api", "example.com/users"); got != "example.com/api/users" {
		t.Fatalf("host pattern = %q, want example.com/api/users", got)
	}
}

// contains reports whether s contains sub (small helper to avoid strings import
// churn in this file).
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
