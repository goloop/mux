package mux

import "strings"

// splitPattern separates an optional leading method token from the rest of a
// ServeMux pattern. In ServeMux grammar the method, when present, is a bare word
// followed by a space and comes before the host/path. Everything after that
// space (host and path) is returned untouched as rest.
//
//	splitPattern("GET /items/{id}")            -> "GET", "/items/{id}"
//	splitPattern("GET example.com/items/{id}") -> "GET", "example.com/items/{id}"
//	splitPattern("/items/{id}")                -> "",    "/items/{id}"
func splitPattern(pattern string) (method, rest string) {
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return pattern[:i], strings.TrimLeft(pattern[i+1:], " ")
	}
	return "", pattern
}

// joinPattern prepends a path prefix to the path portion of rest, leaving any
// host component in place. rest is the host/path part of a pattern (no method).
//
//	joinPattern("/api", "/users")            -> "/api/users"
//	joinPattern("/api", "example.com/users") -> "example.com/api/users"
func joinPattern(prefix, rest string) string {
	if prefix == "" {
		return rest
	}

	// Split an optional host from the path. A ServeMux path always starts with
	// a slash, so the first slash marks where the path begins.
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		// No slash: inside a Route prefix a bare word is treated as a relative
		// path fragment (Route("/api") + "users" -> "/api/users"), a documented
		// convenience. This does mean a host-only string without a path is
		// folded into the path here rather than rejected as it would be at the
		// top level; that ambiguity is accepted so the path-fragment shorthand
		// keeps working.
		return joinPath(prefix, "/"+rest)
	}
	host, path := rest[:slash], rest[slash:]
	return host + joinPath(prefix, path)
}

// joinPath concatenates a prefix and a path with exactly one separating slash,
// preserving the trailing-slash and {$} semantics of the standard mux.
//
//	joinPath("/api", "/users")  -> "/api/users"
//	joinPath("/api/", "/users") -> "/api/users"
//	joinPath("/api", "/")       -> "/api/"
//	joinPath("/api", "/{$}")    -> "/api/{$}"
func joinPath(prefix, path string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	if path == "" {
		return prefix + "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	return prefix + path
}
