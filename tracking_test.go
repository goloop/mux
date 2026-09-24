package mux

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errorBody is what the test error handler writes, so its presence in a
// response proves the error handler ran.
const errorBody = "ERROR-BODY"

// routerWithErrorHandler returns a router whose error handler is visible in
// the response and records whether it ran.
func routerWithErrorHandler(ran *bool) *Router {
	r := New(WithErrorHandler(func(w http.ResponseWriter, req *http.Request, err error) {
		*ran = true
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(errorBody))
	}))
	return r
}

// Flushing commits the response: the implicit 200 is already on its way, so a
// later error must not have an error body appended under it.
func TestFlushCommitsTheResponse(t *testing.T) {
	for _, name := range []string{"ResponseController", "direct Flusher"} {
		t.Run(name, func(t *testing.T) {
			var ran bool
			r := routerWithErrorHandler(&ran)
			r.GetE("/stream", func(w http.ResponseWriter, req *http.Request) error {
				if name == "ResponseController" {
					if err := http.NewResponseController(w).Flush(); err != nil {
						t.Errorf("Flush: %v", err)
					}
				} else {
					f, ok := w.(http.Flusher)
					if !ok {
						t.Fatal("the writer does not expose Flusher")
					}
					f.Flush()
				}
				return errors.New("late failure")
			})

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stream", nil))

			if ran {
				t.Error("the error handler ran after the response was flushed")
			}
			if strings.Contains(w.Body.String(), errorBody) {
				t.Errorf("body = %q, want no error body under a committed status", w.Body)
			}
		})
	}
}

// An informational 1xx does not finish a response: the handler may still fail
// and owe the client a final status. 101 is the exception.
func TestInformationalStatusDoesNotCommit(t *testing.T) {
	cases := []struct {
		name        string
		code        int
		wantHandler bool
	}{
		{"100 Continue", http.StatusContinue, true},
		{"103 Early Hints", http.StatusEarlyHints, true},
		{"101 Switching Protocols", http.StatusSwitchingProtocols, false},
		{"200 OK", http.StatusOK, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ran bool
			r := routerWithErrorHandler(&ran)
			r.GetE("/hints", func(w http.ResponseWriter, req *http.Request) error {
				w.WriteHeader(c.code)
				return errors.New("cannot produce the final response")
			})

			r.ServeHTTP(httptest.NewRecorder(),
				httptest.NewRequest(http.MethodGet, "/hints", nil))

			if ran != c.wantHandler {
				t.Errorf("error handler ran = %v, want %v", ran, c.wantHandler)
			}
		})
	}
}

// The wrapper must expose exactly the optional interfaces the underlying
// writer has: no more, so a handler cannot be told a writer can flush when it
// cannot, and no fewer, so moving a handler to the *E form does not silently
// disable streaming, upgrades or the io.Copy fast path.
func TestWrapperMirrorsWriterCapabilities(t *testing.T) {
	cases := []struct {
		name                    string
		w                       http.ResponseWriter
		flush, hijack, readFrom bool
	}{
		{"plain", plainWriter{}, false, false, false},
		{"flusher", flushWriter{}, true, false, false},
		{"hijacker", hijackWriter{}, false, true, false},
		{"readerFrom", readFromWriter{}, false, false, true},
		{"all three", allWriter{}, true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := newTrackingWriter(c.w)
			if _, ok := got.(http.Flusher); ok != c.flush {
				t.Errorf("Flusher = %v, want %v", ok, c.flush)
			}
			if _, ok := got.(http.Hijacker); ok != c.hijack {
				t.Errorf("Hijacker = %v, want %v", ok, c.hijack)
			}
			if _, ok := got.(io.ReaderFrom); ok != c.readFrom {
				t.Errorf("ReaderFrom = %v, want %v", ok, c.readFrom)
			}
			// Unwrap must always lead back, so ResponseController works.
			u, ok := got.(interface{ Unwrap() http.ResponseWriter })
			if !ok || u.Unwrap() != c.w {
				t.Error("Unwrap does not lead back to the original writer")
			}
		})
	}
}

// Taking the connection over commits the response; a refused hijack does not.
func TestHijackCommitsOnlyWhenItSucceeds(t *testing.T) {
	t.Run("successful", func(t *testing.T) {
		var ran bool
		r := routerWithErrorHandler(&ran)
		r.GetE("/upgrade", func(w http.ResponseWriter, req *http.Request) error {
			if _, _, err := w.(http.Hijacker).Hijack(); err != nil {
				t.Fatalf("Hijack: %v", err)
			}
			return errors.New("after the takeover")
		})
		r.ServeHTTP(hijackWriter{}, httptest.NewRequest(http.MethodGet, "/upgrade", nil))
		if ran {
			t.Error("the error handler ran after the connection was taken over")
		}
	})

	t.Run("refused", func(t *testing.T) {
		var ran bool
		r := routerWithErrorHandler(&ran)
		r.GetE("/upgrade", func(w http.ResponseWriter, req *http.Request) error {
			_, _, err := w.(http.Hijacker).Hijack()
			return err
		})
		r.ServeHTTP(refusingHijackWriter{}, httptest.NewRequest(http.MethodGet, "/upgrade", nil))
		if !ran {
			t.Error("a refused hijack left the error unreported")
		}
	})
}

// Writing the body through ReadFrom commits the response too.
func TestReadFromCommitsTheResponse(t *testing.T) {
	var ran bool
	r := routerWithErrorHandler(&ran)
	r.GetE("/file", func(w http.ResponseWriter, req *http.Request) error {
		if _, err := w.(io.ReaderFrom).ReadFrom(strings.NewReader("body")); err != nil {
			return err
		}
		return errors.New("late failure")
	})
	r.ServeHTTP(readFromWriter{}, httptest.NewRequest(http.MethodGet, "/file", nil))
	if ran {
		t.Error("the error handler ran after the body was written with ReadFrom")
	}
}

// A writer whose FlushError fails must report that error rather than have it
// swallowed by a plain Flush.
func TestFlushErrorIsReported(t *testing.T) {
	sentinel := errors.New("flush failed")
	var got error
	r := New()
	r.GetE("/stream", func(w http.ResponseWriter, req *http.Request) error {
		got = http.NewResponseController(w).Flush()
		return nil
	})
	r.ServeHTTP(failingFlushWriter{err: sentinel},
		httptest.NewRequest(http.MethodGet, "/stream", nil))

	if !errors.Is(got, sentinel) {
		t.Errorf("Flush error = %v, want the writer's own error", got)
	}
}

// Writers with a chosen set of capabilities, for the mirroring test.
type plainWriter struct{}

func (plainWriter) Header() http.Header         { return http.Header{} }
func (plainWriter) Write(b []byte) (int, error) { return len(b), nil }
func (plainWriter) WriteHeader(int)             {}

type flushWriter struct{ plainWriter }

func (flushWriter) Flush() {}

type hijackWriter struct{ plainWriter }

func (hijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, _ := net.Pipe()
	return c, bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c)), nil
}

type refusingHijackWriter struct{ plainWriter }

func (refusingHijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}

type readFromWriter struct{ plainWriter }

func (readFromWriter) ReadFrom(r io.Reader) (int64, error) { return io.Copy(io.Discard, r) }

type allWriter struct{ plainWriter }

func (allWriter) Flush() {}
func (allWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}
func (allWriter) ReadFrom(r io.Reader) (int64, error) { return io.Copy(io.Discard, r) }

type failingFlushWriter struct {
	plainWriter
	err error
}

func (f failingFlushWriter) Flush()            {}
func (f failingFlushWriter) FlushError() error { return f.err }
