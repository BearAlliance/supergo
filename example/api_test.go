package example_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bearalliance/go-super/example"
	"github.com/bearalliance/go-super/pkg/supergo"
)

// newAPI returns a fresh router + store for each test so state never leaks.
func newAPI() *example.Store {
	return example.NewStore()
}

// ── List books ────────────────────────────────────────────────────────────────

func TestListBooksEmpty(t *testing.T) {
	store := newAPI()
	supergo.New(example.NewRouter(store)).
		Get("/books").
		Expect(200).
		ExpectBodyMatchesJSON([]any{}).
		Test(t)
}

func TestListBooksReturnsSeededBooks(t *testing.T) {
	store := newAPI()
	store.Add(example.Book{Title: "The Go Programming Language", Author: "Donovan"})
	store.Add(example.Book{Title: "Clean Code", Author: "Martin"})

	supergo.New(example.NewRouter(store)).
		Get("/books").
		Expect(200).
		ExpectBodyContainsJSON("0.title", "The Go Programming Language").
		ExpectBodyContainsJSON("1.title", "Clean Code").
		Test(t)
}

func TestListBooksFilterByAuthor(t *testing.T) {
	store := newAPI()
	store.Add(example.Book{Title: "Book A", Author: "Smith"})
	store.Add(example.Book{Title: "Book B", Author: "Jones"})

	res := supergo.New(example.NewRouter(store)).
		Get("/books").
		Query("author", "Smith").
		Expect(200).
		Test(t)

	// Only one book should be returned.
	supergo.New(example.NewRouter(store)).
		Get("/books").
		Query("author", "Smith").
		ExpectFn(func(r *supergo.Response) error {
			if !containsOnce(res.Body, "Smith") {
				return fmt.Errorf("expected exactly one Smith book")
			}
			return nil
		}).
		Test(t)
}

// ── Get single book ───────────────────────────────────────────────────────────

func TestGetBookFound(t *testing.T) {
	store := newAPI()
	store.Add(example.Book{Title: "SICP", Author: "Abelson"})

	supergo.New(example.NewRouter(store)).
		Get("/books/1").
		Expect(200).
		ExpectHeader("Content-Type", "application/json").
		ExpectBodyContainsJSON("title", "SICP").
		ExpectBodyContainsJSON("author", "Abelson").
		ExpectBodyContainsJSON("id", float64(1)).
		Test(t)
}

func TestGetBookNotFound(t *testing.T) {
	supergo.New(example.NewRouter(newAPI())).
		Get("/books/999").
		Expect(404).
		ExpectBodyContainsJSON("error", "not found").
		Test(t)
}

func TestGetBookInvalidID(t *testing.T) {
	supergo.New(example.NewRouter(newAPI())).
		Get("/books/abc").
		Expect(400).
		ExpectBodyContainsJSON("error", "invalid id").
		Test(t)
}

// ── Create book (protected) ───────────────────────────────────────────────────

func TestCreateBookUnauthenticated(t *testing.T) {
	supergo.New(example.NewRouter(newAPI())).
		Post("/books").
		SendJSON(example.Book{Title: "Refactoring", Author: "Fowler"}).
		Expect(401).
		ExpectBodyContainsJSON("error", "not authenticated").
		Test(t)
}

func TestCreateBookAuthenticated(t *testing.T) {
	store := newAPI()
	agent := supergo.NewAgent(example.NewRouter(store))

	// Login first — agent persists the session cookie.
	agent.Post("/login").
		SendJSON(map[string]string{"username": "admin", "password": "secret"}).
		Expect(200).
		ExpectBodyContainsJSON("token", "tok-admin").
		Test(t)

	// Create a book — session cookie is sent automatically.
	agent.Post("/books").
		SendJSON(example.Book{Title: "Refactoring", Author: "Fowler"}).
		Expect(201).
		ExpectBodyContainsJSON("title", "Refactoring").
		ExpectBodyContainsJSON("author", "Fowler").
		ExpectBodyContainsJSON("id", float64(1)).
		Test(t)
}

func TestCreateBookBadBody(t *testing.T) {
	agent := supergo.NewAgent(example.NewRouter(newAPI()))

	agent.Post("/login").
		SendJSON(map[string]string{"username": "admin", "password": "secret"}).
		Expect(200).
		Test(t)

	agent.Post("/books").
		Set("Content-Type", "application/json").
		Send("not json at all").
		Expect(400).
		Test(t)
}

// ── Delete book (protected) ───────────────────────────────────────────────────

func TestDeleteBookAuthenticated(t *testing.T) {
	store := newAPI()
	store.Add(example.Book{Title: "Domain-Driven Design", Author: "Evans"})

	agent := supergo.NewAgent(example.NewRouter(store))

	agent.Post("/login").
		SendJSON(map[string]string{"username": "admin", "password": "secret"}).
		Expect(200).
		Test(t)

	agent.Delete("/books/1").
		Expect(204).
		Test(t)

	// Confirm the book is gone.
	agent.Get("/books/1").
		Expect(404).
		Test(t)
}

func TestDeleteBookUnauthenticated(t *testing.T) {
	store := newAPI()
	store.Add(example.Book{Title: "Domain-Driven Design", Author: "Evans"})

	supergo.New(example.NewRouter(store)).
		Delete("/books/1").
		Expect(401).
		Test(t)
}

// ── Login / logout ────────────────────────────────────────────────────────────

func TestLoginInvalidCredentials(t *testing.T) {
	supergo.New(example.NewRouter(newAPI())).
		Post("/login").
		SendJSON(map[string]string{"username": "hacker", "password": "wrong"}).
		Expect(401).
		ExpectBodyContainsJSON("error", "invalid credentials").
		Test(t)
}

func TestLoginMalformedBody(t *testing.T) {
	supergo.New(example.NewRouter(newAPI())).
		Post("/login").
		Set("Content-Type", "application/json").
		Send("not-valid-json").
		Expect(400).
		Test(t)
}

func TestLogoutClearsSession(t *testing.T) {
	store := newAPI()
	agent := supergo.NewAgent(example.NewRouter(store))

	agent.Post("/login").
		SendJSON(map[string]string{"username": "admin", "password": "secret"}).
		Expect(200).
		Test(t)

	// Can access protected resource while logged in.
	store.Add(example.Book{Title: "DDD", Author: "Evans"})
	agent.Post("/books").
		SendJSON(example.Book{Title: "New Book", Author: "Author"}).
		Expect(201).
		Test(t)

	// Logout.
	agent.Post("/logout").
		Expect(204).
		Test(t)

	// Protected resource is now blocked.
	agent.Post("/books").
		SendJSON(example.Book{Title: "Another", Author: "X"}).
		Expect(401).
		Test(t)
}

// ── History inspection ────────────────────────────────────────────────────────

func TestAgentHistoryRecordsFullFlow(t *testing.T) {
	store := newAPI()
	agent := supergo.NewAgent(example.NewRouter(store))

	agent.Post("/login").
		SendJSON(map[string]string{"username": "admin", "password": "secret"}).
		Expect(200).
		Test(t)

	agent.Post("/books").
		SendJSON(example.Book{Title: "Pragmatic Programmer", Author: "Hunt"}).
		Expect(201).
		Test(t)

	agent.Get("/books/1").
		Expect(200).
		Test(t)

	history := agent.History()
	if len(history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(history))
	}

	// Verify each entry's method and status.
	steps := []struct {
		method string
		path   string
		status int
	}{
		{"POST", "/login", 200},
		{"POST", "/books", 201},
		{"GET", "/books/1", 200},
	}
	for i, step := range steps {
		e := history[i]
		if e.Method != step.method {
			t.Errorf("history[%d]: expected method %s, got %s", i, step.method, e.Method)
		}
		if e.Path != step.path {
			t.Errorf("history[%d]: expected path %s, got %s", i, step.path, e.Path)
		}
		if e.Response.StatusCode != step.status {
			t.Errorf("history[%d]: expected status %d, got %d", i, step.status, e.Response.StatusCode)
		}
	}
}

// ── External HTTP dependency (cover service) ──────────────────────────────────

// TestCreateBookFetchesCoverURL demonstrates stubbing an outgoing HTTP call.
// The bookstore calls an external cover service when creating a book; here we
// spin up an httptest.Server as the stub and verify the returned book includes
// the URL the stub provided.
func TestCreateBookFetchesCoverURL(t *testing.T) {
	var receivedQuery string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"url": "https://covers.example.com/go-book.jpg"})
	}))
	defer stub.Close()

	store := newAPI()
	agent := supergo.NewAgent(example.NewRouter(store, stub.URL))

	agent.Post("/login").
		SendJSON(map[string]string{"username": "admin", "password": "secret"}).
		Expect(200).
		Test(t)

	agent.Post("/books").
		SendJSON(example.Book{Title: "The Go Programming Language", Author: "Donovan"}).
		Expect(201).
		ExpectBodyContainsJSON("title", "The Go Programming Language").
		ExpectBodyContainsJSON("cover_url", "https://covers.example.com/go-book.jpg").
		Test(t)

	// Verify the bookstore forwarded title and author to the cover service.
	if !containsOnce([]byte(receivedQuery), "title=") {
		t.Errorf("cover service not called with title param, got query: %s", receivedQuery)
	}
	if !containsOnce([]byte(receivedQuery), "author=") {
		t.Errorf("cover service not called with author param, got query: %s", receivedQuery)
	}
}

// TestCreateBookCoverServiceUnavailable shows graceful degradation: if the
// cover service is down the book is still created, just without a cover URL.
func TestCreateBookCoverServiceUnavailable(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer stub.Close()

	store := newAPI()
	agent := supergo.NewAgent(example.NewRouter(store, stub.URL))

	agent.Post("/login").
		SendJSON(map[string]string{"username": "admin", "password": "secret"}).
		Expect(200).
		Test(t)

	agent.Post("/books").
		SendJSON(example.Book{Title: "Refactoring", Author: "Fowler"}).
		Expect(201).
		ExpectBodyContainsJSON("title", "Refactoring").
		ExpectBodyContainsJSON("author", "Fowler").
		Test(t)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func containsOnce(body []byte, s string) bool {
	count := 0
	b := string(body)
	for i := 0; i < len(b); {
		j := indexString(b[i:], s)
		if j < 0 {
			break
		}
		count++
		i += j + len(s)
	}
	return count == 1
}

func indexString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
