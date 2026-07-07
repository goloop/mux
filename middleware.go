package mux

// Use appends middleware to this router. They apply to every route registered
// afterwards on this router and on sub-routers created from it. As with the
// common middleware convention, call Use before registering the routes it
// should cover: middleware are captured when a route is registered, so a route
// added earlier will not see middleware added later.
func (r *Router) Use(m ...Middleware) {
	r.middlewares = append(r.middlewares, m...)
}

// With returns a new sub-router that carries the parent's middleware plus the
// given extra middleware. The parent is left unchanged, so With is convenient
// for a one-off route:
//
//	r.With(auth).Get("/me", showMe)
func (r *Router) With(m ...Middleware) *Router {
	c := r.clone()
	c.middlewares = append(c.middlewares, m...)
	return c
}

// Group runs fn against a sub-router that inherits the current prefix and
// middleware. Middleware added inside fn stay inside the group and do not leak
// back to the parent:
//
//	r.Group(func(r *mux.Router) {
//	    r.Use(auth)
//	    r.Get("/me", showMe)
//	    r.Post("/logout", logout)
//	})
func (r *Router) Group(fn func(r *Router)) {
	fn(r.clone())
}

// Route runs fn against a sub-router mounted at the given path prefix. The
// prefix is joined onto every route fn registers:
//
//	r.Route("/api/v1", func(r *mux.Router) {
//	    r.Get("/users/{id}", show) // "GET /api/v1/users/{id}"
//	})
func (r *Router) Route(prefix string, fn func(r *Router)) {
	c := r.clone()
	c.prefix = joinPath(r.prefix, prefix)
	fn(c)
}
