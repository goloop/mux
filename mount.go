package mux

import (
	"net/http"
	"strconv"
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
//
// The prefix must be a literal path. A wildcard such as "/orgs/{org}" is a
// routing pattern, not a string that can be removed from a path, and a
// percent-encoded prefix does not equal the decoded path the handler would be
// given; both used to register successfully and then never match, answering
// 404 for every request under the mount. Either is refused here, including a
// wildcard inherited from an enclosing Route.
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
		if bad := unstrippable(base); bad != "" {
			panic("mux: MountStrip prefix " + strconv.Quote(prefix) +
				" cannot be stripped: it contains " + bad +
				", so the mounted handler would never be reached")
		}

		// A request for exactly the prefix ("/admin") would leave StripPrefix
		// with an empty path, which a mounted sub-router cleans to "/" and
		// redirects to the site root, dropping the prefix. Redirect the exact
		// prefix to the subtree root instead, so the stripped handler only ever
		// sees a non-empty path and relative URLs resolve correctly.
		r.register(base, redirectToSubtree(base))
		r.register(base+"/", http.StripPrefix(base, h))
		return
	}

	// Mount (no strip): the handler sees the original, unmodified path, so the
	// exact prefix and the subtree can share it with no empty-path problem and
	// no standard redirect from "/admin" to "/admin/".
	r.register(base, h)
	r.register(base+"/", h)
}

// redirectToSubtree sends a request for exactly the mount prefix on to the
// subtree root, carrying the query with it.
//
// A fixed http.RedirectHandler cannot: it is built once, from a string, and
// knows nothing of the request it answers, so every token, callback and filter
// in the query was dropped on the way to "/panel/".
func redirectToSubtree(base string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		target := base + "/"
		if req.URL.RawQuery != "" || req.URL.ForceQuery {
			// Passed through exactly as it arrived: decoding and re-encoding
			// a query can change what it means.
			target += "?" + req.URL.RawQuery
		}
		http.Redirect(w, req, target, http.StatusTemporaryRedirect)
	})
}

// unstrippable names what makes a prefix impossible to strip, or "" when it
// can be. http.StripPrefix removes a literal string from the path, while the
// ServeMux matches a pattern against the decoded path; where the two disagree
// the mount matches nothing at all.
func unstrippable(base string) string {
	switch {
	case strings.ContainsAny(base, "{}"):
		return "a wildcard"
	case strings.Contains(base, "%"):
		return "a percent-encoded character"
	}
	return ""
}
