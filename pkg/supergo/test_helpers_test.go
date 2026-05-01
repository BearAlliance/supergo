package supergo_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// testMux builds a simple handler used across tests.
func testMux() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"users":[{"name":"alice"},{"name":"bob"}]}`)
	})

	mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(body) //nolint:errcheck
	})

	mux.HandleFunc("GET /echo-header", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		fmt.Fprintf(w, "query=%s", q)
	})

	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "tok123"})
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /profile", func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session")
		if err != nil || c.Value != "tok123" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"name":"alice","role":"admin"}`)
	})

	mux.HandleFunc("DELETE /logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", MaxAge: -1})
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /form", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm() //nolint:errcheck
		fmt.Fprintf(w, "name=%s", r.FormValue("name"))
	})

	return mux
}

// spyT captures Errorf calls without failing the real test, allowing tests to
// assert that a piece of code under test would fail a test.
type spyT struct {
	*testing.T
	errors []string
}

func (s *spyT) Errorf(format string, args ...any) {
	s.errors = append(s.errors, fmt.Sprintf(format, args...))
}
