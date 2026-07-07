// Package mux is a small, predictable routing layer on top of the standard
// net/http.ServeMux. Since Go 1.22 the standard multiplexer already understands
// methods in patterns (GET /posts/{id}), wildcard segments ({id}, {path...}),
// the exact trailing-slash marker {$}, host patterns, Request.PathValue and the
// "most specific wins" precedence rule. mux does not reimplement any of that; it
// adds the ergonomics that the standard library leaves out: method helpers,
// prefix groups, middleware chains and an optional error-returning handler.
//
// The patterns you pass to mux are plain net/http.ServeMux patterns, not a
// custom syntax. Everything you can write in http.ServeMux you can write here.
//
// A Router is itself an http.Handler, so it composes with the rest of net/http:
//
//	r := mux.New()
//	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
//	    w.Write([]byte("ok"))
//	})
//	http.ListenAndServe(":8080", r)
//
// Method helpers register the matching ServeMux pattern:
//
//	r.Get("/users/{id}", show)   // registers "GET /users/{id}"
//
// Route groups share a path prefix and a middleware snapshot:
//
//	r.Route("/api/v1", func(r *mux.Router) {
//	    r.Use(auth)
//	    r.Get("/users/{id}", show)   // "GET /api/v1/users/{id}"
//	})
//
// Middleware model. Middleware registered with Use or With are applied at
// registration time, wrapping the final handler. Because a handler runs after
// routing, such middleware can read path values via mux.Param. Middleware do
// NOT run for unmatched requests (404) or method mismatches (405). For
// application-wide middleware that must run for every request, wrap the whole
// router, which is a plain http.Handler:
//
//	handler := requestID(recoverer(logger(r)))
//	http.ListenAndServe(":8080", handler)
//
// Error handlers. HandlerFunc returns an error and pairs with a central
// ErrorHandler, which is convenient with the goloop/resp package:
//
//	r := mux.New(mux.WithErrorHandler(func(w http.ResponseWriter, req *http.Request, err error) {
//	    resp.Error(w, http.StatusInternalServerError, "internal server error")
//	}))
//	r.GetE("/users/{id}", func(w http.ResponseWriter, req *http.Request) error {
//	    return resp.JSON(w, resp.R{"id": mux.Param(req, "id")})
//	})
//
// Concurrency. Register all routes before the server starts serving. Like
// http.ServeMux, a Router is safe for concurrent reads (serving) once routes
// are in place, but registering routes concurrently with serving is not
// supported.
package mux
