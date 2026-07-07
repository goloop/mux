package mux

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve is a small helper: it builds a request, runs it through r and returns
// the recorded response.
func serve(r http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestMethodHelpersRegisterServeMuxPatterns(t *testing.T) {
	r := New()
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, Param(req, "id"))
	})

	rec := serve(r, http.MethodGet, "/users/42")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "42" {
		t.Fatalf("body = %q, want %q", got, "42")
	}
}

func TestGetMatchesHead(t *testing.T) {
	r := New()
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-Seen", "yes")
	})

	rec := serve(r, http.MethodHead, "/x")
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200 (GET must answer HEAD)", rec.Code)
	}
	if rec.Header().Get("X-Seen") != "yes" {
		t.Fatalf("GET handler did not run for HEAD")
	}
}

func TestMethodNotAllowedDefault(t *testing.T) {
	r := New()
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {})

	rec := serve(r, http.MethodPost, "/x")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Fatalf("Allow = %q, want it to contain GET", allow)
	}
}

func TestMethodNotAllowedCustom(t *testing.T) {
	r := New(WithMethodNotAllowed(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			fmt.Fprint(w, "nope")
		})))
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {})

	rec := serve(r, http.MethodDelete, "/x")
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 from custom handler", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Fatalf("Allow = %q, want GET preserved before custom handler", allow)
	}
	if rec.Body.String() != "nope" {
		t.Fatalf("body = %q, want custom body", rec.Body.String())
	}
}

func TestCustomNotFound(t *testing.T) {
	r := New(WithNotFound(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusGone)
			fmt.Fprint(w, "gone")
		})))
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {})

	rec := serve(r, http.MethodGet, "/missing")
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410 from custom not-found", rec.Code)
	}
	if rec.Body.String() != "gone" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "gone")
	}
}

func TestRoutePrefixJoin(t *testing.T) {
	cases := []struct {
		name      string
		prefix    string
		path      string
		reqTarget string
	}{
		{"simple", "/api", "/users", "/api/users"},
		{"prefix trailing slash", "/api/", "/users", "/api/users"},
		{"path without leading slash is normalized", "/api", "users", "/api/users"},
		{"param path", "/api", "/users/{id}", "/api/users/7"},
		{"exact trailing slash marker", "/api", "/{$}", "/api/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New()
			hit := false
			r.Route(tc.prefix, func(r *Router) {
				r.Get(tc.path, func(w http.ResponseWriter, req *http.Request) {
					hit = true
				})
			})
			rec := serve(r, http.MethodGet, tc.reqTarget)
			if rec.Code != http.StatusOK || !hit {
				t.Fatalf("target %q: status=%d hit=%v, want 200 and hit",
					tc.reqTarget, rec.Code, hit)
			}
		})
	}
}

func TestNestedRoutePrefix(t *testing.T) {
	r := New()
	r.Route("/api", func(r *Router) {
		r.Route("/v1", func(r *Router) {
			r.Get("/ping", func(w http.ResponseWriter, req *http.Request) {
				fmt.Fprint(w, "pong")
			})
		})
	})
	rec := serve(r, http.MethodGet, "/api/v1/ping")
	if rec.Code != http.StatusOK || rec.Body.String() != "pong" {
		t.Fatalf("status=%d body=%q, want 200 pong", rec.Code, rec.Body.String())
	}
}

// tagMW records the order in which middleware run by appending to a shared
// slice through a response header trail.
func tagMW(tag string, trail *[]string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			*trail = append(*trail, tag)
			next.ServeHTTP(w, req)
		})
	}
}

func TestMiddlewareOrder(t *testing.T) {
	var trail []string
	r := New()
	r.Use(tagMW("A", &trail))
	r.Group(func(r *Router) {
		r.Use(tagMW("C", &trail))
		r.With(tagMW("B", &trail)).Get("/x", func(w http.ResponseWriter, req *http.Request) {
			trail = append(trail, "handler")
		})
	})

	serve(r, http.MethodGet, "/x")
	got := strings.Join(trail, ",")
	// Use(A) on root, Use(C) in group, With(B) for the route: outermost first.
	if got != "A,C,B,handler" {
		t.Fatalf("order = %q, want %q", got, "A,C,B,handler")
	}
}

func TestWithDoesNotMutateParent(t *testing.T) {
	var trail []string
	r := New()
	r.Use(tagMW("root", &trail))

	// Build a child with an extra middleware, but register a route on the
	// parent afterwards: the child's middleware must not leak into it.
	_ = r.With(tagMW("child", &trail))
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {})

	serve(r, http.MethodGet, "/x")
	if strings.Join(trail, ",") != "root" {
		t.Fatalf("trail = %v, want only [root]; child middleware leaked", trail)
	}
}

func TestHandleErrorUsesErrorHandler(t *testing.T) {
	r := New(WithErrorHandler(func(w http.ResponseWriter, req *http.Request, err error) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, err.Error())
	}))
	r.GetE("/boom", func(w http.ResponseWriter, req *http.Request) error {
		return fmt.Errorf("kaboom")
	})

	rec := serve(r, http.MethodGet, "/boom")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 from error handler", rec.Code)
	}
	if rec.Body.String() != "kaboom" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "kaboom")
	}
}

func TestHandleErrorNilDoesNothing(t *testing.T) {
	r := New(WithErrorHandler(func(w http.ResponseWriter, req *http.Request, err error) {
		t.Fatalf("error handler must not run when handler returns nil")
	}))
	r.GetE("/ok", func(w http.ResponseWriter, req *http.Request) error {
		fmt.Fprint(w, "fine")
		return nil
	})

	rec := serve(r, http.MethodGet, "/ok")
	if rec.Code != http.StatusOK || rec.Body.String() != "fine" {
		t.Fatalf("status=%d body=%q, want 200 fine", rec.Code, rec.Body.String())
	}
}

func TestDefaultErrorHandlerJSON(t *testing.T) {
	r := New() // no WithErrorHandler: default resp JSON 500
	r.GetE("/boom", func(w http.ResponseWriter, req *http.Request) error {
		return fmt.Errorf("kaboom")
	})

	rec := serve(r, http.MethodGet, "/boom")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}
	if body := rec.Body.String(); strings.Contains(body, "kaboom") {
		t.Fatalf("default handler leaked the error text: %q", body)
	}
}

func TestPanicOnConflictingPatterns(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic on conflicting patterns")
		}
	}()
	r := New()
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {})
	r.Get("/x", func(w http.ResponseWriter, req *http.Request) {}) // duplicate -> panic
}

func TestHandleWithMethodInPattern(t *testing.T) {
	r := New()
	r.Handle("GET /raw/{id}", http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprint(w, Param(req, "id"))
		}))
	rec := serve(r, http.MethodGet, "/raw/9")
	if rec.Code != http.StatusOK || rec.Body.String() != "9" {
		t.Fatalf("status=%d body=%q, want 200 9", rec.Code, rec.Body.String())
	}
}

func TestMount(t *testing.T) {
	// Without strip the mounted handler sees the full, unmodified path.
	echo := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, req.URL.Path)
	})

	r := New()
	r.Mount("/admin", echo)

	rec := serve(r, http.MethodGet, "/admin/ping")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "/admin/ping" {
		t.Fatalf("body = %q, want full path /admin/ping", rec.Body.String())
	}

	// The exact prefix is served too, with no redirect.
	rec = serve(r, http.MethodGet, "/admin")
	if rec.Code != http.StatusOK || rec.Body.String() != "/admin" {
		t.Fatalf("exact mount: status=%d body=%q, want 200 /admin",
			rec.Code, rec.Body.String())
	}
}

func TestMountStrip(t *testing.T) {
	sub := New()
	sub.Get("/ping", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, "pong")
	})

	r := New()
	r.MountStrip("/admin", sub)

	rec := serve(r, http.MethodGet, "/admin/ping")
	if rec.Code != http.StatusOK || rec.Body.String() != "pong" {
		t.Fatalf("status=%d body=%q, want 200 pong", rec.Code, rec.Body.String())
	}
}

func TestChainOrder(t *testing.T) {
	var trail []string
	h := Chain(tagMW("a", &trail), tagMW("b", &trail))(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			trail = append(trail, "h")
		}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Join(trail, ",") != "a,b,h" {
		t.Fatalf("order = %v, want a,b,h", trail)
	}
}

func TestRouterIsHTTPHandler(t *testing.T) {
	var _ http.Handler = New()
}
