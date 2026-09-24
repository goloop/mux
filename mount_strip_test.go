package mux

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The redirect from the exact mount prefix to its subtree root must carry the
// query: a token, a callback or a filter dropped there is data the client
// sent and the handler never sees.
func TestMountStripRedirectKeepsQuery(t *testing.T) {
	cases := []struct {
		name, url, want string
	}{
		{"query", "/panel?token=abc&next=%2Fhome", "/panel/?token=abc&next=%2Fhome"},
		{"no query", "/panel", "/panel/"},
		{"empty forced query", "/panel?", "/panel/?"},
		{"encoding preserved", "/panel?q=a%20b&r=%2B", "/panel/?q=a%20b&r=%2B"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := New()
			r.MountStrip("/panel", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.url, nil))

			if w.Code != http.StatusTemporaryRedirect {
				t.Fatalf("status = %d, want 307", w.Code)
			}
			if got := w.Header().Get("Location"); got != c.want {
				t.Errorf("Location = %q, want %q", got, c.want)
			}
		})
	}
}

// Below the prefix nothing redirects, and the mounted handler sees the path
// relative to its own root.
func TestMountStripServesTheSubtree(t *testing.T) {
	var seen string
	r := New()
	r.MountStrip("/panel", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seen = req.URL.Path + "?" + req.URL.RawQuery
	}))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panel/users?page=2", nil))

	if seen != "/users?page=2" {
		t.Errorf("handler saw %q, want %q", seen, "/users?page=2")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// A prefix that cannot be stripped is refused at registration, instead of
// registering successfully and then answering 404 for the whole mount.
func TestMountStripRejectsUnstrippablePrefix(t *testing.T) {
	cases := map[string]func(*Router){
		"wildcard": func(r *Router) {
			r.MountStrip("/orgs/{org}", http.NotFoundHandler())
		},
		"percent-encoded": func(r *Router) {
			r.MountStrip("/caf%C3%A9", http.NotFoundHandler())
		},
		"wildcard inherited from Route": func(r *Router) {
			r.Route("/orgs/{org}", func(r *Router) {
				r.MountStrip("/files", http.NotFoundHandler())
			})
		},
	}
	for name, register := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				v := recover()
				if v == nil {
					t.Fatal("registration was accepted")
				}
				if msg, ok := v.(string); !ok || !strings.Contains(msg, "MountStrip") {
					t.Errorf("panic = %v, want it to name MountStrip", v)
				}
			}()
			register(New())
		})
	}
}

// Mount, which passes the original path through, still accepts a wildcard
// prefix: there is nothing to strip, so nothing to disagree about.
func TestMountStillAcceptsWildcardPrefix(t *testing.T) {
	var org string
	r := New()
	r.Mount("/orgs/{org}", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		org = req.PathValue("org")
	}))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/orgs/acme/files", nil))

	if w.Code != http.StatusOK || org != "acme" {
		t.Errorf("status=%d org=%q, want 200 and acme", w.Code, org)
	}
}
