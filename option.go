package mux

import "net/http"

// Option configures a Router at construction time. Options are applied by New
// in order.
type Option func(*Router)

// WithErrorHandler sets the ErrorHandler used to render errors returned by the
// error-returning helpers (GetE, PostE, HandleError and so on). Without it, a
// returned error yields a generic JSON 500 via goloop/resp.
//
//	r := mux.New(mux.WithErrorHandler(func(w http.ResponseWriter, req *http.Request, err error) {
//	    resp.Error(w, http.StatusInternalServerError, "internal server error")
//	}))
func WithErrorHandler(h ErrorHandler) Option {
	return func(r *Router) { r.errorHandler = h }
}

// WithNotFound sets the handler used when no route matches the request path.
// Without it, the standard http.NotFound reply is used.
func WithNotFound(h http.Handler) Option {
	return func(r *Router) { r.notFound = h }
}

// WithMethodNotAllowed sets the handler used when the path matches but the
// method does not. The Allow header computed by the standard mux is preserved
// on the response before the handler runs. Without it, a plain 405 is returned.
func WithMethodNotAllowed(h http.Handler) Option {
	return func(r *Router) { r.methodNotAllowed = h }
}
