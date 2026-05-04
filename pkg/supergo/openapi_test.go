package supergo_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bearalliance/supergo/example"
	"github.com/bearalliance/supergo/pkg/supergo"
)

func TestLoadOpenAPISpecAndMatchBookstoreRoute(t *testing.T) {
	spec := supergo.MustOpenAPISpec(filepath.Join("..", "..", "example", "openapi.yaml"))

	store := example.NewStore()
	store.Add(example.Book{Title: "SICP", Author: "Abelson"})

	supergo.New(example.NewRouter(store)).
		Get("/books/1").
		Expect(200).
		ExpectMatchesSpec(spec).
		Test(t)
}

func TestExpectMatchesSpecWithPathParameterInference(t *testing.T) {
	spec := supergo.MustOpenAPISpec(filepath.Join("..", "..", "example", "openapi.yaml"))

	store := example.NewStore()
	agent := supergo.NewAgent(example.NewRouter(store))

	agent.Post("/login").
		SendJSON(map[string]string{"username": "admin", "password": "secret"}).
		Expect(200).
		Test(t)

	agent.Post("/books").
		SendJSON(example.Book{Title: "Refactoring", Author: "Fowler"}).
		Expect(201).
		ExpectMatchesSpec(spec).
		Test(t)
}

func TestExpectMatchesSpecReportsSchemaMismatch(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(specPath, []byte(`
openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /users:
    get:
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                additionalProperties: false
                required: [users]
                properties:
                  users:
                    type: array
                    items:
                      type: string
`), 0o600); err != nil {
		t.Fatalf("writing temp OpenAPI spec: %v", err)
	}

	spec := supergo.MustOpenAPISpec(specPath)
	spy := &spyT{T: t}

	supergo.New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"wrong":true}`))
	})).
		Get("/users").
		ExpectMatchesSpec(spec).
		Test(spy)

	if len(spy.errors) == 0 {
		t.Fatal("expected OpenAPI mismatch to be reported")
	}
	if !strings.Contains(spy.errors[0], `missing required property "users"`) {
		t.Fatalf("expected missing-property error, got: %v", spy.errors)
	}
}

func TestMustOpenAPISpecPanicsOnMissingFile(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for missing OpenAPI spec")
		}
	}()
	_ = supergo.MustOpenAPISpec(filepath.Join(t.TempDir(), "missing.yaml"))
}
