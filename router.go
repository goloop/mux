package mux

import (
	"net/http"
	"slices"
)

// Router is an ergonomic wrapper around http.ServeMux. It is an http.Handler,
// so it can be served directly or mounted inside another handler. Sub-routers
// created by Route, Group and With share the same underlying ServeMux and add
// their own path prefix and middleware snapshot.
type Router struct {
	// mux is the single, shared ServeMux that actually performs matching. Every
	// sub-router points at the same instance.
	mux *http.ServeMux

	// root points at the top-level Router. Options such as the custom 404/405
	// handlers live there and are consulted from ServeHTTP.
	root *Router

	// prefix is the accumulated path prefix applied to every pattern this
	// router registers.
	prefix string

	// middlewares is the snapshot applied, in order, to handlers registered on
	// this router. Sub-routers receive a clone so that appends never leak.
	middlewares []Middleware

	// errorHandler renders errors returned by HandlerFunc (the *E helpers).
	errorHandler ErrorHandler

	// notFound and methodNotAllowed override the standard replies. They are
	// only meaningful on the root router and are read in ServeHTTP.
	notFound         http.Handler
	methodNotAllowed http.Handler
}

// New creates a Router with a fresh ServeMux and applies the given options.
func New(opts ...Option) *Router {
	r := &Router{mux: http.NewServeMux()}
	r.root = r
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Handle registers a handler for a full ServeMux pattern. The pattern may carry
// a method and host, exactly as http.ServeMux accepts (for example
// "GET example.com/items/{id}"). The router's prefix is applied to the path.
func (r *Router) Handle(pattern string, h http.Handler) {
	method, rest := splitPattern(pattern)
	full := joinPattern(r.prefix, rest)
	if method != "" {
		full = method + " " + full
	}
	r.register(full, h)
}

// HandleFunc is Handle for an http.HandlerFunc.
func (r *Router) HandleFunc(pattern string, h http.HandlerFunc) {
	r.Handle(pattern, h)
}

// Method registers a handler for the given method and path. The path is joined
// with the router prefix; the method must not be embedded in path.
func (r *Router) Method(method, path string, h http.HandlerFunc) {
	r.register(method+" "+joinPattern(r.prefix, path), h)
}

// Get registers h for "GET path". Per the standard ServeMux, a GET route also
// answers HEAD requests.
func (r *Router) Get(path string, h http.HandlerFunc) { r.Method(http.MethodGet, path, h) }

// Post registers h for "POST path".
func (r *Router) Post(path string, h http.HandlerFunc) { r.Method(http.MethodPost, path, h) }

// Put registers h for "PUT path".
func (r *Router) Put(path string, h http.HandlerFunc) { r.Method(http.MethodPut, path, h) }

// Patch registers h for "PATCH path".
func (r *Router) Patch(path string, h http.HandlerFunc) { r.Method(http.MethodPatch, path, h) }

// Delete registers h for "DELETE path".
func (r *Router) Delete(path string, h http.HandlerFunc) { r.Method(http.MethodDelete, path, h) }

// Options registers h for "OPTIONS path".
func (r *Router) Options(path string, h http.HandlerFunc) { r.Method(http.MethodOptions, path, h) }

// Head registers h for "HEAD path". This is rarely needed, since a GET route
// already answers HEAD; use it only for a HEAD-specific handler.
func (r *Router) Head(path string, h http.HandlerFunc) { r.Method(http.MethodHead, path, h) }

// register wraps h with the current middleware snapshot and installs it on the
// shared ServeMux. Wrapping happens once, at registration time.
func (r *Router) register(pattern string, h http.Handler) {
	r.mux.Handle(pattern, r.wrap(h))
}

// wrap applies this router's middleware to h from innermost to outermost, so
// the first middleware in the slice ends up as the outermost wrapper.
func (r *Router) wrap(h http.Handler) http.Handler {
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		h = r.middlewares[i](h)
	}
	return h
}

// clone returns a shallow copy that shares the ServeMux and root but owns an
// independent middleware slice, so appends on the copy never touch the parent.
func (r *Router) clone() *Router {
	c := *r
	c.middlewares = slices.Clone(r.middlewares)
	return &c
}

// ServeHTTP dispatches req through the shared ServeMux. When custom 404 or 405
// handlers are configured, it inspects the standard reply first and substitutes
// the configured handler; otherwise it delegates directly with no overhead.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	root := r.root
	if root.notFound == nil && root.methodNotAllowed == nil {
		root.mux.ServeHTTP(w, req)
		return
	}

	// ServeMux answers "OPTIONS *" (RequestURI == "*") with 400 before any
	// routing; Handler(req) does not, so delegate to keep that behaviour.
	if req.RequestURI == "*" {
		root.mux.ServeHTTP(w, req)
		return
	}

	h, pattern := root.mux.Handler(req)
	if pattern != "" {
		// Delegate to ServeMux.ServeHTTP, not the returned handler: only the
		// former fills the request's path values and Pattern, so PathValue/Param
		// keep working when a custom 404/405 handler is configured.
		root.mux.ServeHTTP(w, req)
		return
	}

	// No pattern matched: the standard handler replies with either 404, 405 or
	// an internal redirect (e.g. for a "dirty" path that cleans to an unmatched
	// route). Run it against a sniffer to learn which, discarding its output.
	sn := &sniffer{header: make(http.Header)}
	h.ServeHTTP(sn, req)

	// Anything other than 404/405 (a redirect, a 400) is a genuine standard
	// response; replay it against the real writer instead of overriding it.
	if sn.status != http.StatusNotFound &&
		sn.status != http.StatusMethodNotAllowed {
		root.mux.ServeHTTP(w, req)
		return
	}

	if sn.status == http.StatusMethodNotAllowed {
		if allow := sn.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		if root.methodNotAllowed != nil {
			root.methodNotAllowed.ServeHTTP(w, req)
			return
		}
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
		return
	}

	if root.notFound != nil {
		root.notFound.ServeHTTP(w, req)
		return
	}
	http.NotFound(w, req)
}

// sniffer is a throwaway ResponseWriter used only on the unmatched path to read
// the status and Allow header the standard mux would have written. It discards
// the body and never touches the real client connection.
type sniffer struct {
	header http.Header
	status int
}

// Header exposes the captured header map. WriteHeader records the status the
// standard mux would have sent, and Write reports the body as written while
// discarding it, so the sniffer fully satisfies http.ResponseWriter without
// touching the client.
func (s *sniffer) Header() http.Header         { return s.header }
func (s *sniffer) WriteHeader(code int)        { s.status = code }
func (s *sniffer) Write(b []byte) (int, error) { return len(b), nil }
