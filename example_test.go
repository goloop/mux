package mux_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/goloop/mux"
	"github.com/goloop/resp/v2"
)

// ExampleRouter shows a minimal router with a path parameter.
func ExampleRouter() {
	r := mux.New()
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "user %s", mux.Param(req, "id"))
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42", nil))
	fmt.Println(rec.Body.String())
	// Output: user 42
}

// ExampleRouter_Route groups routes under a shared prefix.
func ExampleRouter_Route() {
	r := mux.New()
	r.Route("/api/v1", func(r *mux.Router) {
		r.Get("/ping", func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprint(w, "pong")
		})
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil))
	fmt.Println(rec.Body.String())
	// Output: pong
}

// ExampleRouter_GetE shows an error-returning handler backed by goloop/resp.
func ExampleRouter_GetE() {
	r := mux.New(mux.WithErrorHandler(
		func(w http.ResponseWriter, req *http.Request, err error) {
			resp.Error(w, http.StatusInternalServerError, "internal server error")
		}))

	r.GetE("/users/{id}", func(w http.ResponseWriter, req *http.Request) error {
		return resp.JSON(w, resp.R{"id": mux.Param(req, "id")})
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/7", nil))
	fmt.Println(rec.Body.String())
	// Output: {"id":"7"}
}
