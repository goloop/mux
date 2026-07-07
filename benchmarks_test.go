package mux

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func noop(w http.ResponseWriter, r *http.Request) {}

func passMW(next http.Handler) http.Handler { return next }

// BenchmarkStdServeMux is the baseline: the standard library on its own.
func BenchmarkStdServeMux(b *testing.B) {
	m := http.NewServeMux()
	m.HandleFunc("GET /users/{id}", noop)
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.ServeHTTP(rec, req)
	}
}

// BenchmarkRouterNoMiddleware measures the wrapper overhead over the baseline.
func BenchmarkRouterNoMiddleware(b *testing.B) {
	r := New()
	r.Get("/users/{id}", noop)
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.ServeHTTP(rec, req)
	}
}

// BenchmarkRouterFiveMiddleware measures the cost of a five-deep chain.
func BenchmarkRouterFiveMiddleware(b *testing.B) {
	r := New()
	r.Use(passMW, passMW, passMW, passMW, passMW)
	r.Get("/users/{id}", noop)
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.ServeHTTP(rec, req)
	}
}

// BenchmarkRouterErrorHandler measures the error-returning handler path where
// the handler succeeds (nil error, no error-handler dispatch).
func BenchmarkRouterErrorHandler(b *testing.B) {
	r := New()
	r.GetE("/users/{id}", func(w http.ResponseWriter, req *http.Request) error {
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.ServeHTTP(rec, req)
	}
}
