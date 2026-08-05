package mux_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/goloop/mux"
)

// lineBelow returns the file and line of the statement that follows the call,
// in the form the router puts into a registration panic. Tests use it to name
// the exact registration they expect to be blamed.
func lineBelow() string {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d", file, line+1)
}

// registrationPanic runs fn and returns the message it panicked with.
func registrationPanic(t *testing.T, fn func()) string {
	t.Helper()
	var msg string
	func() {
		defer func() {
			if v := recover(); v != nil {
				msg = fmt.Sprint(v)
			}
		}()
		fn()
	}()
	if msg == "" {
		t.Fatal("registration did not panic")
	}
	return msg
}

func nop(http.ResponseWriter, *http.Request) {}

// TestConflictNamesBothCallSites is the point of the whole mechanism: a route
// conflict has to name the two lines in the application, not the one line
// inside the router that every registration passes through.
func TestConflictNamesBothCallSites(t *testing.T) {
	r := mux.New()

	first := lineBelow()
	r.Get("/conversations/{id}/messages", nop)

	var second string
	msg := registrationPanic(t, func() {
		second = lineBelow()
		r.Get("/conversations/messages/{id}", nop)
	})

	for _, want := range []string{first, second} {
		if !strings.Contains(msg, want) {
			t.Errorf("panic does not name %s:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "router.go:") {
		t.Errorf("panic still points inside the router:\n%s", msg)
	}
	// The standard explanation must survive intact: it is the part that says
	// why the two patterns cannot be ordered.
	if !strings.Contains(msg, "both match some paths") {
		t.Errorf("standard conflict explanation was lost:\n%s", msg)
	}
	if !strings.HasPrefix(msg, "mux: pattern ") {
		t.Errorf("unexpected shape:\n%s", msg)
	}
}

// TestConflictThroughRouteAndMount checks the call site is found whatever the
// depth of router helpers between the application and the standard mux.
func TestConflictThroughRouteAndMount(t *testing.T) {
	r := mux.New()

	var inner string
	r.Route("/api", func(r *mux.Router) {
		inner = lineBelow()
		r.Get("/items/{id}", nop)
	})

	var outer string
	msg := registrationPanic(t, func() {
		outer = lineBelow()
		r.Get("/api/items/{name}", nop)
	})

	for _, want := range []string{inner, outer} {
		if !strings.Contains(msg, want) {
			t.Errorf("panic does not name %s:\n%s", want, msg)
		}
	}
}

// TestMountConflictNamesCallSite covers the deepest helper chain: Mount
// registers two patterns of its own, one level further from the caller.
func TestMountConflictNamesCallSite(t *testing.T) {
	r := mux.New()

	mounted := lineBelow()
	r.Mount("/admin", http.HandlerFunc(nop))

	var clash string
	msg := registrationPanic(t, func() {
		clash = lineBelow()
		r.Handle("/admin/", http.HandlerFunc(nop))
	})

	for _, want := range []string{mounted, clash} {
		if !strings.Contains(msg, want) {
			t.Errorf("panic does not name %s:\n%s", want, msg)
		}
	}
}

// TestDuplicateReportsFirstRegistration checks that registering the same
// pattern twice blames the original registration, not the newcomer twice.
func TestDuplicateReportsFirstRegistration(t *testing.T) {
	r := mux.New()

	first := lineBelow()
	r.Get("/items", nop)

	var second string
	msg := registrationPanic(t, func() {
		second = lineBelow()
		r.Get("/items", nop)
	})

	if !strings.Contains(msg, first) {
		t.Errorf("panic does not name the first registration %s:\n%s", first, msg)
	}
	if !strings.Contains(msg, second) {
		t.Errorf("panic does not name the second registration %s:\n%s", second, msg)
	}
}

// TestMalformedPatternNamesCallSite covers the other half: a registration that
// fails for its own reasons has no second route to point at, so the call site
// is prefixed instead and the standard message is kept whole.
func TestMalformedPatternNamesCallSite(t *testing.T) {
	r := mux.New()

	var site string
	msg := registrationPanic(t, func() {
		site = lineBelow()
		r.Get("/items/{bad", nop)
	})

	if want := "mux: " + site + ": "; !strings.HasPrefix(msg, want) {
		t.Errorf("want prefix %q, got:\n%s", want, msg)
	}
	if !strings.Contains(msg, "{bad") {
		t.Errorf("standard message was lost:\n%s", msg)
	}
}

// TestRegistrationUnchanged is the boring half of the contract: routes that do
// not conflict register and match exactly as before.
func TestRegistrationUnchanged(t *testing.T) {
	r := mux.New()
	r.Get("/a", nop)
	r.Post("/a", nop)
	r.Get("/a/{id}", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, mux.Param(req, "id"))
	})
	r.Mount("/admin", http.HandlerFunc(nop))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/a/7", nil))
	if got := w.Body.String(); got != "7" {
		t.Errorf("path value = %q, want %q", got, "7")
	}
}
