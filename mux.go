package mux

import (
	"net/http"

	"github.com/goloop/resp/v2"
)

// Middleware is the standard net/http middleware shape: it wraps a handler and
// returns a new handler. Any function of this type - including those from the
// goloop/middlewares package or third-party libraries - works with a Router.
type Middleware func(http.Handler) http.Handler

// HandlerFunc is an http handler that returns an error. It is not a replacement
// for http.HandlerFunc; it is an optional adapter for code that writes
// responses through goloop/resp and wants a single place to turn errors into
// responses. A non-nil error is passed to the Router's ErrorHandler.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// ErrorHandler renders an error returned by a HandlerFunc. It is set once per
// Router with WithErrorHandler. When none is configured, the Router uses a
// default that replies with a JSON 500 via goloop/resp.
type ErrorHandler func(http.ResponseWriter, *http.Request, error)

// Param returns the path value bound to name for the current request. It is a
// thin alias for r.PathValue(name), provided so handlers written against mux
// have one obvious entry point without importing anything extra.
func Param(r *http.Request, name string) string {
	return r.PathValue(name)
}

// Chain composes middleware into a single Middleware. The first middleware in
// the list is the outermost wrapper, so it runs first on the way in and last on
// the way out:
//
//	mw := mux.Chain(a, b, c) // request flows a -> b -> c -> handler
func Chain(m ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(m) - 1; i >= 0; i-- {
			next = m[i](next)
		}
		return next
	}
}

// defaultErrorHandler is used when a Router has no ErrorHandler configured. It
// keeps the error text out of the response and replies with a generic JSON 500.
func defaultErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	resp.Error(w, http.StatusInternalServerError, "internal server error")
}
