package mux

import (
	"net/http"
	"testing"
)

// namedMiddleware stands in for any other package's defined middleware type -
// goloop/middlewares or a third-party one. Before Middleware became an alias,
// passing such a value required an explicit conversion the caller had to
// discover; two independent projects hit that boundary in the same day.
type namedMiddleware func(http.Handler) http.Handler

func TestForeignMiddlewareTypesAreAssignable(t *testing.T) {
	called := false
	mw := namedMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			next.ServeHTTP(w, r)
		})
	})

	// The assignment below, with no conversion, is the point of the alias.
	var m Middleware = mw
	h := m(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(nil, nil)
	if !called {
		t.Fatal("middleware did not run")
	}
}
