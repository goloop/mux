package mux

import "net/http"

// toHandlerFunc adapts an error-returning HandlerFunc to a plain
// http.HandlerFunc. The error handler is resolved once, at registration time:
// the router's ErrorHandler if set, otherwise the package default.
func (r *Router) toHandlerFunc(h HandlerFunc) http.HandlerFunc {
	eh := r.errorHandler
	if eh == nil {
		eh = defaultErrorHandler
	}
	return func(w http.ResponseWriter, req *http.Request) {
		tw := &trackingWriter{ResponseWriter: w}
		if err := h(tw, req); err != nil && !tw.wrote {
			// Only invoke the error handler when nothing has been written yet.
			// If the handler already sent status or body and then returned an
			// error, calling the error handler would corrupt the response with
			// a second WriteHeader and extra bytes. Return an error before
			// writing anything.
			eh(w, req, err)
		}
	}
}

// trackingWriter records whether a response has been started, so the error
// handler is skipped once the wrapped handler has already written. It forwards
// Unwrap so http.ResponseController still reaches the underlying writer's
// Flusher, Hijacker and deadline methods.
type trackingWriter struct {
	http.ResponseWriter
	wrote bool
}

func (t *trackingWriter) WriteHeader(code int) {
	t.wrote = true
	t.ResponseWriter.WriteHeader(code)
}

func (t *trackingWriter) Write(b []byte) (int, error) {
	t.wrote = true
	return t.ResponseWriter.Write(b)
}

func (t *trackingWriter) Unwrap() http.ResponseWriter { return t.ResponseWriter }

// HandleError registers an error-returning handler for a full ServeMux pattern.
func (r *Router) HandleError(pattern string, h HandlerFunc) {
	r.HandleFunc(pattern, r.toHandlerFunc(h))
}

// MethodE registers an error-returning handler for the given method and path.
func (r *Router) MethodE(method, path string, h HandlerFunc) {
	r.Method(method, path, r.toHandlerFunc(h))
}

// GetE registers an error-returning handler for "GET path".
func (r *Router) GetE(path string, h HandlerFunc) { r.MethodE(http.MethodGet, path, h) }

// PostE registers an error-returning handler for "POST path".
func (r *Router) PostE(path string, h HandlerFunc) { r.MethodE(http.MethodPost, path, h) }

// PutE registers an error-returning handler for "PUT path".
func (r *Router) PutE(path string, h HandlerFunc) { r.MethodE(http.MethodPut, path, h) }

// PatchE registers an error-returning handler for "PATCH path".
func (r *Router) PatchE(path string, h HandlerFunc) { r.MethodE(http.MethodPatch, path, h) }

// DeleteE registers an error-returning handler for "DELETE path".
func (r *Router) DeleteE(path string, h HandlerFunc) { r.MethodE(http.MethodDelete, path, h) }
