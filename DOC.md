# mux - reference

`mux` is a thin ergonomic layer over `net/http.ServeMux`. This document covers
the full API and the behavioural contracts you should know before relying on it.

## Table of contents

- [Mental model](#mental-model)
- [Constructing a router](#constructing-a-router)
- [Registering routes](#registering-routes)
- [Path parameters](#path-parameters)
- [Route groups and prefixes](#route-groups-and-prefixes)
- [Middleware](#middleware)
- [Error-returning handlers](#error-returning-handlers)
- [Mounting handlers](#mounting-handlers)
- [Custom 404 and 405](#custom-404-and-405)
- [Concurrency](#concurrency)

## Mental model

This is `net/http.ServeMux` with a thin ergonomic layer on top. Matching, precedence,
method handling, `HEAD` for `GET`, host patterns, path escaping and redirects
are all done by the standard library. `mux` only:

1. joins a path prefix onto the patterns you register,
2. wraps handlers with a middleware snapshot at registration time,
3. adds an optional error-returning handler shape.

The patterns are standard `net/http` patterns. There is no custom pattern
syntax, no regular expressions and no reverse routing.

## Constructing a router

```go
r := mux.New()

r := mux.New(
    mux.WithErrorHandler(myErrorHandler),
    mux.WithNotFound(myNotFound),
    mux.WithMethodNotAllowed(myMethodNotAllowed),
)
```

Options are applied in order. A `Router` is an `http.Handler`.

## Registering routes

Method helpers register `METHOD path`:

```go
r.Get("/users/{id}", h)     // GET /users/{id}
r.Post("/users", h)         // POST /users
r.Put("/users/{id}", h)
r.Patch("/users/{id}", h)
r.Delete("/users/{id}", h)
r.Options("/users", h)
r.Head("/users/{id}", h)    // rarely needed: GET already answers HEAD
r.Method("REPORT", "/x", h) // any other method
```

`Handle` and `HandleFunc` accept a full pattern that may include a method and a
host:

```go
r.Handle("GET example.com/x", h)
r.HandleFunc("GET /x", h)
```

A `GET` route also answers `HEAD`, because the standard mux says so. Do not add
method checks of your own.

Registering two handlers for the same pattern panics, exactly as `net/http`
does. This surfaces developer mistakes early; it is not caught or turned into an
error in v0.

## Path parameters

Read parameters with the standard accessor or its alias:

```go
id := r.PathValue("id")   // standard
id := mux.Param(r, "id")  // identical, one obvious name
```

## Route groups and prefixes

```go
r.Route("/api/v1", func(r *mux.Router) {
    r.Get("/users/{id}", show)   // GET /api/v1/users/{id}
    r.Route("/admin", func(r *mux.Router) {
        r.Get("/stats", stats)   // GET /api/v1/admin/stats
    })
})
```

`Group` is `Route` without an added prefix - it exists to scope middleware:

```go
r.Group(func(r *mux.Router) {
    r.Use(auth)
    r.Get("/me", showMe)
    r.Post("/logout", logout)
})
```

Prefix joining normalizes slashes: `/api` and `/api/` behave the same, a path
without a leading slash gets one, and `/{$}` (exact match) is preserved.

## Middleware

A middleware is `func(http.Handler) http.Handler`.

- `Use(m...)` appends middleware to the current router. They apply to routes
  registered afterwards and to sub-routers created from it.
- `With(m...)` returns a sub-router with extra middleware, leaving the parent
  unchanged - handy for a single route: `r.With(auth).Get("/me", h)`.
- `Chain(m...)` composes middleware into one.

Middleware are captured when a route is registered, so route-scoped middleware
can read path values. **They do not run for `404` or `405`.** For app-wide
middleware that must observe every request, wrap the router itself:

```go
http.ListenAndServe(":8080", mux.Chain(requestID, recoverer, logger)(r))
```

Order: the first middleware is the outermost wrapper. `Use(A)`, then a group
with `Use(C)`, then `With(B)` on the route runs `A -> C -> B -> handler`.

`With` never mutates its parent: middleware added to a child do not leak back.

## Error-returning handlers

`HandlerFunc` returns an error:

```go
type HandlerFunc func(http.ResponseWriter, *http.Request) error
```

Register with the `E` helpers:

```go
r.GetE("/users/{id}", func(w http.ResponseWriter, req *http.Request) error {
    u, err := load(mux.Param(req, "id"))
    if err != nil {
        return err
    }
    return resp.JSON(w, u)
})
```

A non-nil error goes to the router's `ErrorHandler`. If none is set, the default
replies with a generic JSON `500` via `goloop/resp` and does not leak the error
text. A nil error means the handler already wrote the response; nothing extra
happens.

Available: `GetE`, `PostE`, `PutE`, `PatchE`, `DeleteE`, `MethodE`,
`HandleError`.

## Mounting handlers

```go
r.Mount("/admin", adminHandler)      // serves /admin and /admin/...
r.MountStrip("/admin", adminRouter)  // same, but strips /admin first
```

- `Mount` passes the original request path to the handler. Use it for handlers
  that expect the full path.
- `MountStrip` applies `http.StripPrefix`, so a self-contained handler or
  sub-router routes relative to its own root.

Both register the exact prefix and the subtree separately, so there is no
redirect from `/admin` to `/admin/`.

## Custom 404 and 405

```go
r := mux.New(
    mux.WithNotFound(notFound),
    mux.WithMethodNotAllowed(methodNotAllowed),
)
```

When a method mismatch occurs, the `Allow` header the standard mux computes is
copied onto the response before your handler runs. When neither option is set,
the standard `404`/`405` replies are used and there is zero extra overhead on the
matched path.

## Concurrency

Register every route before the server starts serving. Like `http.ServeMux`, a
`Router` is safe for concurrent serving once routes are in place; registering
routes concurrently with serving is not supported.
