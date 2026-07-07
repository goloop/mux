# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Initial v0 development branch: a small ergonomic layer over `net/http.ServeMux`.

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
