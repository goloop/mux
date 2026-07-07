package mux

import (
	"net/http"
	"strings"
)

// Mount attaches an http.Handler at a path prefix, serving both the prefix
// itself and everything beneath it. The mounted handler sees the original,
// unmodified request path; use MountStrip if it expects the prefix removed.
// The router's middleware snapshot is applied to the mounted handler.
//
//	r.Mount("/admin", adminHandler) // serves /admin and /admin/...
func (r *Router) Mount(prefix string, h http.Handler) {
	r.mount(prefix, h, false)
}

// MountStrip is Mount with the mount prefix stripped from the request URL before
// the handler runs, via http.StripPrefix. Use it for a self-contained handler
// that routes relative to its own root.
func (r *Router) MountStrip(prefix string, h http.Handler) {
	r.mount(prefix, h, true)
}

func (r *Router) mount(prefix string, h http.Handler, strip bool) {
	base := strings.TrimSuffix(joinPath(r.prefix, prefix), "/")
	if base == "" {
		// Mount at the site root: a single subtree pattern covers everything.
		r.register("/", h)
		return
	}

	target := h
	if strip {
		target = http.StripPrefix(base, h)
	}

	// Register the exact prefix and the subtree separately so the standard mux
	// does not emit a redirect from "/admin" to "/admin/".
	r.register(base, target)
	r.register(base+"/", target)
}
