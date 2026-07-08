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

	if strip {
		// A request for exactly the prefix ("/admin") would leave StripPrefix
		// with an empty path, which a mounted sub-router cleans to "/" and
		// redirects to the site root, dropping the prefix. Redirect the exact
		// prefix to the subtree root instead, so the stripped handler only ever
		// sees a non-empty path and relative URLs resolve correctly.
		r.register(base, http.RedirectHandler(base+"/", http.StatusTemporaryRedirect))
		r.register(base+"/", http.StripPrefix(base, h))
		return
	}

	// Mount (no strip): the handler sees the original, unmodified path, so the
	// exact prefix and the subtree can share it with no empty-path problem and
	// no standard redirect from "/admin" to "/admin/".
	r.register(base, h)
	r.register(base+"/", h)
}
