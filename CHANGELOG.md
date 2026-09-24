# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.2.0] - 2026-09-24

Minor release: the custom 404/405 path stops rebuilding itself on every
request, and reports this router's own routing state.

### Fixed
- The middleware chain for a custom `404`/`405` reply is built once instead of
  on every unmatched request. Rebuilding it meant every middleware constructor
  ran again per request: state a middleware set up while wrapping was thrown
  away between replies, a constructor that starts a worker started one per
  request, and constructors written for sequential registration suddenly ran
  concurrently. Fifty concurrent unmatched requests used to run the
  constructor fifty-one times; they now run it once.
- A custom fallback reports this router's match state. When a `Router` is
  mounted inside another `ServeMux`, a request that matched something out
  there (`/tenant/{tenant}/{rest...}`) carried that pattern and its path
  values into the fallback, so logging, metrics or middleware reading
  `Pattern` or `Param` saw a match this router never made. They are empty
  there now, as they already were on the standard path, and the extra work is
  skipped entirely when the request carries no outer match.

### Changed
- The `Allow` header of a method mismatch is set before the fallback chain
  runs rather than inside it, so root middleware can see it. A custom handler
  can still replace it.
- `doc.go` and the README said middleware do not run for `404`/`405`, while
  the reference said they do. Both are half right and now say so: they run for
  a custom reply, not for the standard one.

### Performance
- The custom fallback path got cheaper as a side effect: with five middleware,
  a `405` went from about 1290 ns, 904 B and 26 allocations per reply to about
  1130 ns, 736 B and 20, and a `404` from about 970 ns and 744 B to about
  840 ns and 624 B.

## [1.1.1] - 2026-08-11

Patch release.

### Changed
- Depends on `resp/v2` v2.3.0, which adds the machine-readable `error` field to
  error bodies; error responses this router renders through resp can now carry
  a slug.

### Documentation
- The reference states that `Middleware` is a type alias and shows what that
  buys: values from `goloop/middlewares` or any third-party package of the same
  shape pass to `Use` with no conversion.

## [1.0.2] - 2026-08-05

### Documentation
- The package documentation and the README describe the route-conflict panic:
  which two lines it names, and why they are not the ones the standard mux
  would have reported.

## [1.0.1] - 2026-08-05

### Changed
- A route conflict now names the two lines in the application that registered
  the clashing patterns, instead of the single line inside this package that
  every route passes through on its way to the standard mux. The standard
  explanation of why the patterns clash is kept word for word; only the two
  locations are corrected. Any other registration failure, such as a malformed
  pattern, is reported as `mux: <file>:<line>: <standard message>`. A message
  this package cannot parse - a future wording change in the standard library -
  is passed through untouched rather than half-rewritten.

## [1.0.0] - 2026-07-12

The `0.2.0` tree promoted to a stable tag; no code changes.

## [0.2.0] - 2026-07-12

### Changed
- Custom `404` and `405` responses now run through the router-level middleware
  chain (added with `Use` on the router from `New`), so security headers, CORS
  and logging cover error replies too. Previously these handlers were invoked
  directly and bypassed middleware. Middleware may therefore run without a
  matched route, where `PathValue`/`Param` are empty.

### Fixed
- An error-returning handler (`GetE` and friends) that already wrote a response
  and then returns an error no longer has the error handler write a second,
  corrupting response; the error handler is skipped once anything was written.
  The wrapping writer forwards `Unwrap`, so `http.ResponseController` still
  reaches the underlying `Flusher`/`Hijacker`.
- A zero-value `Router{}` (not built with `New`) panics with a clear
  "use mux.New()" message instead of a raw nil dereference.

### Documentation
- Documented that a `Route` prefix folds a slashless token into the path, so a
  host pattern must include a slash (`example.com/`).

## [0.1.2] - 2026-07-10

### Documentation
- Documented the internal `sniffer` ResponseWriter methods used on the
  unmatched-route path.

## [0.1.0]

Initial v0 release: a small ergonomic layer over `net/http.ServeMux`.

### Added

- `Router` type and `New` constructor; a `Router` is an `http.Handler`.
- Method helpers `Get`, `Post`, `Put`, `Patch`, `Delete`, `Options`, `Head`,
  and the generic `Method`; plus `Handle` and `HandleFunc` for full patterns.
- Route composition: `Route`, `Group`, `With`, `Use`, and the `Chain` helper.
- `Mount` and `MountStrip` for attaching handlers at a prefix.
- Error-returning handlers `GetE`, `PostE`, `PutE`, `PatchE`, `DeleteE`,
  `MethodE`, `HandleError`, backed by a configurable `ErrorHandler`. The default
  replies with a JSON 500 via `github.com/goloop/resp`.
- `Param` alias over `Request.PathValue`.
- Options `WithErrorHandler`, `WithNotFound`, `WithMethodNotAllowed`.

### Fixed

- A configured custom 404/405 handler no longer breaks `Request.PathValue` /
  `Param` on matched routes: matched requests are now dispatched through
  `ServeMux.ServeHTTP`, which fills the path values, instead of the bare handler
  returned by `Handler`.
- With a custom 404/405, `OPTIONS *` again returns the standard `400 Bad
  Request`, and a "dirty" path that cleans to an unmatched route again gets the
  standard redirect instead of the custom 404.
- `MountStrip` now redirects a request for exactly the mount prefix (`/panel`)
  to the subtree root (`/panel/`) instead of stripping it to an empty path,
  which a mounted sub-router would bounce to the site root.
