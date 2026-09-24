package mux

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// toHandlerFunc adapts an error-returning HandlerFunc to a plain
// http.HandlerFunc. The error handler is resolved once, at registration time:
// the router's ErrorHandler if set, otherwise the package default.
func (r *Router) toHandlerFunc(h HandlerFunc) http.HandlerFunc {
	eh := r.errorHandler
	if eh == nil {
		eh = defaultErrorHandler
	}
	return func(w http.ResponseWriter, req *http.Request) {
		tracked, tw := newTrackingWriter(w)
		if err := h(tracked, req); err != nil && !tw.wrote {
			// Only invoke the error handler when nothing has been committed
			// yet. If the handler already sent status or body and then
			// returned an error, calling the error handler would corrupt the
			// response with a second WriteHeader and extra bytes. Return an
			// error before writing anything.
			eh(w, req, err)
		}
	}
}

// trackingWriter records whether a response has been committed, so the error
// handler is skipped once the wrapped handler has already answered. It
// forwards Unwrap so http.ResponseController still reaches the underlying
// writer's Flusher, Hijacker and deadline methods.
//
// Committing is more than Write and WriteHeader: flushing sends the implicit
// 200, and a successful hijack hands the connection over entirely. Both used
// to go unnoticed, so a handler that flushed and then failed had an error body
// appended under a status that was already on the wire.
type trackingWriter struct {
	http.ResponseWriter
	wrote bool
}

// WriteHeader records the status unless it is informational. A 1xx does not
// finish a response - the handler may still fail and produce a final status,
// which is what Early Hints are for - so it must not disable the error
// handler. 101 is the exception: switching protocols ends the exchange, which
// is how net/http treats it too.
func (t *trackingWriter) WriteHeader(code int) {
	if code < 100 || code > 199 || code == http.StatusSwitchingProtocols {
		t.wrote = true
	}
	t.ResponseWriter.WriteHeader(code)
}

func (t *trackingWriter) Write(b []byte) (int, error) {
	t.wrote = true
	return t.ResponseWriter.Write(b)
}

func (t *trackingWriter) Unwrap() http.ResponseWriter { return t.ResponseWriter }

// flushError commits the response and flushes it, preferring the underlying
// writer's FlushError so its error is not swallowed.
func (t *trackingWriter) flushError() error {
	t.wrote = true
	if fe, ok := t.ResponseWriter.(interface{ FlushError() error }); ok {
		return fe.FlushError()
	}
	t.ResponseWriter.(http.Flusher).Flush()
	return nil
}

// hijack hands the connection over. Only a successful hijack commits the
// response; a refused one leaves the error handler free to answer.
func (t *trackingWriter) hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, rw, err := t.ResponseWriter.(http.Hijacker).Hijack()
	if err == nil {
		t.wrote = true
	}
	return c, rw, err
}

// readFrom writes the body straight from r, keeping whatever fast path the
// underlying writer has (sendfile, for one).
func (t *trackingWriter) readFrom(r io.Reader) (int64, error) {
	t.wrote = true
	return t.ResponseWriter.(io.ReaderFrom).ReadFrom(r)
}

// The variants below re-expose exactly the optional interfaces the underlying
// writer has. A handler moved from Get to GetE otherwise lost them: a direct
// w.(http.Flusher) assertion failed, so streaming quietly stopped streaming,
// an upgrade could not hijack, and io.Copy lost its fast path.
//
// Which interfaces a writer has cannot be guessed, only mirrored: declaring
// Hijacker on HTTP/2, or Flusher on a writer that cannot flush, would be a
// different lie. Each variant is one pointer wide, so putting it in an
// interface allocates nothing beyond the tracking writer itself.
type (
	twF   struct{ *trackingWriter }
	twH   struct{ *trackingWriter }
	twR   struct{ *trackingWriter }
	twFH  struct{ *trackingWriter }
	twFR  struct{ *trackingWriter }
	twHR  struct{ *trackingWriter }
	twFHR struct{ *trackingWriter }
)

func (w twF) Flush()              { _ = w.flushError() }
func (w twF) FlushError() error   { return w.flushError() }
func (w twFH) Flush()             { _ = w.flushError() }
func (w twFH) FlushError() error  { return w.flushError() }
func (w twFR) Flush()             { _ = w.flushError() }
func (w twFR) FlushError() error  { return w.flushError() }
func (w twFHR) Flush()            { _ = w.flushError() }
func (w twFHR) FlushError() error { return w.flushError() }

func (w twH) Hijack() (net.Conn, *bufio.ReadWriter, error)   { return w.hijack() }
func (w twFH) Hijack() (net.Conn, *bufio.ReadWriter, error)  { return w.hijack() }
func (w twHR) Hijack() (net.Conn, *bufio.ReadWriter, error)  { return w.hijack() }
func (w twFHR) Hijack() (net.Conn, *bufio.ReadWriter, error) { return w.hijack() }

func (w twR) ReadFrom(r io.Reader) (int64, error)   { return w.readFrom(r) }
func (w twFR) ReadFrom(r io.Reader) (int64, error)  { return w.readFrom(r) }
func (w twHR) ReadFrom(r io.Reader) (int64, error)  { return w.readFrom(r) }
func (w twFHR) ReadFrom(r io.Reader) (int64, error) { return w.readFrom(r) }

// newTrackingWriter wraps w so the response can be tracked, in a value that
// implements the same optional interfaces w does.
func newTrackingWriter(w http.ResponseWriter) (http.ResponseWriter, *trackingWriter) {
	t := &trackingWriter{ResponseWriter: w}

	_, flusher := w.(http.Flusher)
	_, hijacker := w.(http.Hijacker)
	_, readerFrom := w.(io.ReaderFrom)

	switch {
	case flusher && hijacker && readerFrom:
		return twFHR{t}, t
	case flusher && hijacker:
		return twFH{t}, t
	case flusher && readerFrom:
		return twFR{t}, t
	case hijacker && readerFrom:
		return twHR{t}, t
	case flusher:
		return twF{t}, t
	case hijacker:
		return twH{t}, t
	case readerFrom:
		return twR{t}, t
	}
	return t, t
}

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
