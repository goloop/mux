package mux

import (
	"net/http"
	"slices"
	"sync"
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

	// reg records where each pattern was registered from, so a conflict can
	// name the application's lines. Sub-routers share the one instance.
	reg *registry

	// fb holds the prepared 404 and 405 chains. It is a pointer because
	// clone copies a Router by value and a sync.Once must not be copied;
	// every router in a tree shares the root's one instance.
	fb *fallbacks
}

// fallbacks holds the handlers used when no route matched, built once and
// then only served.
//
// They used to be assembled inside ServeHTTP, which meant every middleware
// constructor ran again on every 404 and 405: whatever state a middleware set
// up per wrapping was thrown away between requests, a constructor that starts
// a worker started one per request, and constructors written for sequential
// registration suddenly ran concurrently.
type fallbacks struct {
	once       sync.Once
	notFound   http.Handler
	notAllowed http.Handler
}

// New creates a Router with a fresh ServeMux and applies the given options.
func New(opts ...Option) *Router {
	r := &Router{mux: http.NewServeMux(), reg: &registry{}, fb: &fallbacks{}}
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
//
// The standard mux panics on a conflicting or malformed pattern and blames the
// caller of Handle, which is this line for every route in the application.
// The real caller is captured first and put back into the message on the way
// out; see registry.retarget.
func (r *Router) register(pattern string, h http.Handler) {
	if r.mux == nil {
		panic("mux: Router must be created with mux.New()")
	}

	site := callSite()
	if failure := r.install(pattern, h); failure != nil {
		// Raised here, and not inside the function that recovered it, so the
		// runtime prints one panic - the corrected one - rather than chaining
		// it after the standard mux's original with its misleading location.
		panic(r.reg.retarget(failure, site))
	}

	// Recorded only once the registration has succeeded, so that a pattern
	// registered twice still reports the first call site as the one the
	// second registration collides with.
	r.reg.remember(pattern, site)
}

// install hands the wrapped handler to the standard mux, returning the value
// it panicked with instead of letting that panic escape.
func (r *Router) install(pattern string, h http.Handler) (failure any) {
	defer func() { failure = recover() }()
	r.mux.Handle(pattern, r.wrap(h))
	return nil
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
	if r.root == nil || r.mux == nil {
		panic("mux: Router must be created with mux.New()")
	}
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

	// Dispatch the error response through the root middleware chain, so
	// controls added with Use (security headers, CORS, logging) cover 404/405
	// replies too, not only matched routes. Middleware may therefore run
	// without a matched route: request path values are empty on this path.
	// The chains are built once, on the first reply that needs them.
	fb := root.fallbackChains()
	if sn.status == http.StatusMethodNotAllowed {
		// Allow belongs to this request, so it is written on this request's
		// writer rather than carried into a chain shared by all of them. The
		// middleware therefore sees it, and the handler can still change it.
		if allow := sn.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		fb.notAllowed.ServeHTTP(w, req)
		return
	}
	fb.notFound.ServeHTTP(w, req)
}

// fallbackChains prepares the 404 and 405 handlers, once. It is called from
// the serving path, so a router configured and then served concurrently still
// builds each chain exactly once; middleware added after the first fallback
// reply does not reach these chains, which is the documented contract that
// configuration is finished before serving begins.
func (r *Router) fallbackChains() *fallbacks {
	r.fb.once.Do(func() {
		notFound := r.notFound
		if notFound == nil {
			notFound = http.HandlerFunc(http.NotFound)
		}

		notAllowed := r.methodNotAllowed
		if notAllowed == nil {
			notAllowed = http.HandlerFunc(
				func(w http.ResponseWriter, req *http.Request) {
					http.Error(w, http.StatusText(http.StatusMethodNotAllowed),
						http.StatusMethodNotAllowed)
				})
		}
		r.fb.notFound = freshMatch(r.wrap(notFound))
		r.fb.notAllowed = freshMatch(r.wrap(notAllowed))
	})
	return r.fb
}

// freshMatch returns h behind a ServeMux of its own, so that a request
// reaching it carries this router's match state rather than the one it came
// in with.
//
// It matters when a Router is mounted inside another ServeMux: a request that
// matched "/tenant/{tenant}/{rest...}" out there still carried that pattern
// and its path values into the fallback, so anything reading Pattern or Param
// saw a match this router never made. A pattern and its wildcard values are
// private to net/http and cannot be cleared from the outside; dispatching
// through a mux that matches everything and names nothing is what replaces
// them, exactly as the ordinary serving path does.
func freshMatch(h http.Handler) http.Handler {
	m := http.NewServeMux()
	m.Handle("/", http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			// The dispatch above leaves Pattern as "/", which is this mux's
			// business, not a route the application registered.
			req.Pattern = ""
			h.ServeHTTP(w, req)
		}))

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Pattern == "" {
			// Nothing matched this request before it arrived, so there is no
			// foreign match state to replace and no reason to pay for a
			// second dispatch. This is the ordinary case: a router serving
			// at the top of a server.
			h.ServeHTTP(w, req)
			return
		}
		m.ServeHTTP(w, req)
	})
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
