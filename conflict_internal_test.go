package mux

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// TestRetargetFallsBack pins the safety valve. The conflict message belongs to
// the standard library, so this package must degrade to the original text
// rather than hand back a half-rewritten one when the message stops matching.
func TestRetargetFallsBack(t *testing.T) {
	g := &registry{}
	g.remember("GET /a", "app.go:10")

	const site = "app.go:20"
	conflict := `pattern "GET /b" (registered at mux/router.go:100) ` +
		`conflicts with pattern "GET /a" (registered at mux/router.go:100):` +
		"\nbecause reasons"

	cases := []struct {
		name string
		in   any
		want string
	}{
		{
			name: "rewritten when both sites are known",
			in:   errors.New(conflict),
			want: `mux: pattern "GET /b" (registered at app.go:20) ` +
				`conflicts with pattern "GET /a" (registered at app.go:10):` +
				"\nbecause reasons",
		},
		{
			name: "unknown other pattern keeps the original",
			in: errors.New(`pattern "GET /b" (registered at x) conflicts ` +
				`with pattern "GET /never-registered" (registered at x):`),
			want: "mux: " + site + `: pattern "GET /b" (registered at x) ` +
				`conflicts with pattern "GET /never-registered" ` +
				`(registered at x):`,
		},
		{
			name: "reworded message keeps the original",
			in:   errors.New("pattern GET /b clashes with pattern GET /a"),
			want: "mux: " + site + ": pattern GET /b clashes with pattern GET /a",
		},
		{
			name: "message without locations keeps the original",
			in: errors.New(`pattern "GET /b" conflicts with pattern ` +
				`"GET /a" somewhere`),
			want: "mux: " + site + `: pattern "GET /b" conflicts with ` +
				`pattern "GET /a" somewhere`,
		},
		{
			name: "plain failure is prefixed with the call site",
			in:   errors.New("bad wildcard segment"),
			want: "mux: " + site + ": bad wildcard segment",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := g.retarget(c.in, site)
			err, ok := got.(error)
			if !ok {
				t.Fatalf("retarget returned %T, want error", got)
			}
			if err.Error() != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", err, c.want)
			}
		})
	}
}

// TestRetargetPassesThroughNonErrors checks that a panic this package did not
// cause - a nil map write inside a middleware constructor, say - reaches the
// application exactly as it was raised.
func TestRetargetPassesThroughNonErrors(t *testing.T) {
	g := &registry{}
	for _, v := range []any{"boom", 42, struct{}{}} {
		if got := g.retarget(v, "app.go:20"); got != v {
			t.Errorf("retarget(%v) = %v, want it unchanged", v, got)
		}
	}
}

// TestRetargetWithoutCallSite covers a stack this package could not read: with
// nothing to add, the original panic value is left alone.
func TestRetargetWithoutCallSite(t *testing.T) {
	g := &registry{}
	err := errors.New("bad wildcard segment")
	if got := g.retarget(err, ""); got != any(err) {
		t.Errorf("retarget = %v, want the original error", got)
	}
}

// TestSelfPackage checks the prefix the frame walk skips was read correctly;
// an empty or wrong one would silently report this package as the call site.
func TestSelfPackage(t *testing.T) {
	if !strings.HasSuffix(selfPackage, "/mux.") {
		t.Errorf("selfPackage = %q, want it to end in /mux.", selfPackage)
	}
}

// TestSubRoutersShareTheRegistry checks every way of deriving a router carries
// the same record of call sites. A sub-router with its own would forget where
// its routes came from, and a later conflict would name only one of the two.
func TestSubRoutersShareTheRegistry(t *testing.T) {
	r := New()
	subs := map[string]*Router{"With": r.With()}
	r.Route("/api", func(c *Router) { subs["Route"] = c })
	r.Group(func(c *Router) { subs["Group"] = c })

	for name, c := range subs {
		if c.reg != r.reg {
			t.Errorf("%s returned a router with its own registry", name)
		}
	}
}

// TestRegisterIsRaceFree covers routes registered from several goroutines, the
// way a large application assembles its route table from parallel packages.
func TestRegisterIsRaceFree(t *testing.T) {
	r := New()
	h := func(w http.ResponseWriter, req *http.Request) {}

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Get(fmt.Sprintf("/route-%d", i), h)
		}()
	}
	wg.Wait()

	r.reg.mu.Lock()
	got := len(r.reg.sites)
	r.reg.mu.Unlock()
	if got != 16 {
		t.Errorf("recorded %d call sites, want 16", got)
	}
}
