[![Go Report Card](https://goreportcard.com/badge/github.com/goloop/mux)](https://goreportcard.com/report/github.com/goloop/mux) [![License](https://img.shields.io/badge/license-MIT-brightgreen)](https://github.com/goloop/mux/blob/master/LICENSE) [![License](https://img.shields.io/badge/godoc-YES-green)](https://pkg.go.dev/github.com/goloop/mux) [![Stay with Ukraine](https://img.shields.io/static/v1?label=Stay%20with&message=Ukraine%20♥&color=ffD700&labelColor=0057B8&style=flat)](https://u24.gov.ua/)

# mux

`mux` is a small routing layer on top of Go's standard `net/http.ServeMux`.
Since Go 1.22 the standard multiplexer already matches methods, path wildcards
(`{id}`, `{path...}`), the exact trailing-slash marker `{$}`, host patterns and
the "most specific wins" precedence rule. `mux` does not reimplement any of that
- it adds the ergonomics the standard library leaves out: method helpers, prefix
groups, middleware chains and an optional error-returning handler that pairs with
[goloop/resp](https://github.com/goloop/resp).

The patterns you write are plain `net/http.ServeMux` patterns, not a custom
syntax. A `Router` is itself an `http.Handler`, so it drops into any `net/http`
server and composes with any middleware of the form
`func(http.Handler) http.Handler`.

## Features

- Method helpers - `Get`, `Post`, `Put`, `Patch`, `Delete`, `Options`, `Head`,
  and the generic `Method`.
- Route groups and prefixes - `Route`, `Group`, `Mount`, `MountStrip`.
- Middleware chains - `Use`, `With`, `Chain`, applied once at registration time.
- Error-returning handlers - `GetE`/`PostE`/... plus a central `ErrorHandler`,
  with a JSON 500 default via `goloop/resp`.
- Path parameters through the standard `r.PathValue`, aliased as `mux.Param`.
- Custom `404` and `405` handlers via options; the standard `Allow` header is
  preserved.

## Installation

```bash
go get -u github.com/goloop/mux
```

```go
import "github.com/goloop/mux"
```

Requires Go 1.25 or newer.

## Quick start

```go
r := mux.New()

r.Use(requestID, recoverer, logger) // any func(http.Handler) http.Handler

r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
    fmt.Fprintf(w, "user %s", mux.Param(req, "id"))
})

r.Route("/api/v1", func(r *mux.Router) {
    r.Use(auth)
    r.Get("/orders/{id}", showOrder)
    r.Post("/orders", createOrder)
})

http.ListenAndServe(":8080", r)
```

### Working with goloop/resp

Error-returning handlers keep the happy path clean and route failures through a
single place:

```go
r := mux.New(mux.WithErrorHandler(func(w http.ResponseWriter, req *http.Request, err error) {
    resp.Error(w, http.StatusInternalServerError, "internal server error")
}))

r.GetE("/users/{id}", func(w http.ResponseWriter, req *http.Request) error {
    return resp.JSON(w, resp.R{"id": mux.Param(req, "id")})
})
```

Without `WithErrorHandler`, a returned error yields a generic JSON `500` through
`goloop/resp`.

## Patterns are Go ServeMux patterns

`mux` never parses paths itself. Everything you can write in the standard
multiplexer works here unchanged:

| You write                   | Registered pattern         |
|-----------------------------|----------------------------|
| `r.Get("/users/{id}", h)`   | `GET /users/{id}`          |
| `r.Get("/files/{path...}")` | `GET /files/{path...}`     |
| `r.Get("/exact/{$}", h)`    | `GET /exact/{$}`           |
| `r.Handle("GET a.com/x", h)`| `GET a.com/x`              |

Because the standard mux applies "most specific wins", route registration order
does not change matching. Conflicting patterns panic, exactly as `net/http` does.

## Middleware model

Middleware added with `Use` or `With` are captured when a route is registered,
so they can read path values via `mux.Param`. They do **not** run for unmatched
requests (`404`) or method mismatches (`405`). For application-wide middleware
that must run for every request, wrap the router - it is a plain `http.Handler`:

```go
handler := requestID(recoverer(logger(r)))
http.ListenAndServe(":8080", handler)
```

Call `Use` before registering the routes it should cover: a route added earlier
will not see middleware added later.

## Documentation

- Full reference and recipes: [DOC.md](DOC.md) · [DOC.UK.md](DOC.UK.md)
- Package API: [pkg.go.dev/github.com/goloop/mux](https://pkg.go.dev/github.com/goloop/mux)
- Changes between versions: [CHANGELOG.md](CHANGELOG.md)

## Contributing

Contributions are welcome. Please run `go test ./...`, `go vet ./...` and
`gofmt -l .` before submitting a pull request.

## License

`mux` is released under the MIT License. See [LICENSE](LICENSE).
