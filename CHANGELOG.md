# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
