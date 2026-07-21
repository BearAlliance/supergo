package supergo_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/bearalliance/supergo/pkg/supergo"
)

// inMemoryStoreHandler is a tiny stateful fake: POST /items creates an item and
// assigns an id, GET /items/{id} returns it. It is ordinary Go with no supergo
// dependency, standing in for a real external service's behavior.
func inMemoryStoreHandler() http.Handler {
	mux := http.NewServeMux()
	var mu sync.Mutex
	items := map[int]map[string]any{}
	nextID := 1

	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		mu.Lock()
		id := nextID
		nextID++
		body["id"] = id
		items[id] = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(body) //nolint:errcheck
	})

	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		mu.Lock()
		item, ok := items[id]
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"error": "not found"}) //nolint:errcheck
			return
		}
		json.NewEncoder(w).Encode(item) //nolint:errcheck
	})

	return mux
}

func TestFakeStatefulBehavior(t *testing.T) {
	fake := supergo.NewFake(t, inMemoryStoreHandler())

	resp, err := http.Post(fake.URL+"/items", "application/json", strings.NewReader(`{"name":"widget"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	// A later GET reflects the state mutated by the POST: this is the behavior
	// that distinguishes a fake from a stub's canned responses.
	got, err := http.Get(fake.URL + "/items/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer got.Body.Close()
	body, _ := io.ReadAll(got.Body)
	if !strings.Contains(string(body), "widget") {
		t.Errorf("expected GET to reflect the created item, got %s", body)
	}
}

func TestFakeCapturesRequests(t *testing.T) {
	fake := supergo.NewFake(t, inMemoryStoreHandler())

	http.Post(fake.URL+"/items", "application/json", strings.NewReader(`{"name":"a"}`)) //nolint:errcheck

	reqs := fake.Received("POST", "/items")
	if len(reqs) != 1 {
		t.Fatalf("expected 1 captured request, got %d", len(reqs))
	}
	if !strings.Contains(string(reqs[0].Body), `"a"`) {
		t.Errorf("expected captured body to contain the payload, got %s", reqs[0].Body)
	}
}

func TestFakeMustBeCalledPasses(t *testing.T) {
	fake := supergo.NewFake(t, inMemoryStoreHandler()).MustBeCalled("POST", "/items")

	http.Post(fake.URL+"/items", "application/json", strings.NewReader(`{"name":"a"}`)) //nolint:errcheck
}

func TestFakeMustBeCalledFails(t *testing.T) {
	spy := &spyT{T: t}

	t.Cleanup(func() {
		if len(spy.errors) == 0 {
			t.Error("expected MustBeCalled to record an error for an uncalled route")
		}
	})

	supergo.NewFake(spy, inMemoryStoreHandler()).MustBeCalled("GET", "/never")
}

func TestFakeMustBeCalledTimesFails(t *testing.T) {
	spy := &spyT{T: t}

	t.Cleanup(func() {
		if len(spy.errors) == 0 {
			t.Error("expected MustBeCalledTimes to record an error when called the wrong number of times")
		}
	})

	fake := supergo.NewFake(spy, inMemoryStoreHandler()).MustBeCalledTimes("POST", "/items", 2)

	http.Post(fake.URL+"/items", "application/json", strings.NewReader(`{"name":"a"}`)) //nolint:errcheck
}

// widgetSpec writes a minimal OpenAPI spec declaring GET /widget and returns it.
func widgetSpec(t *testing.T) *supergo.OpenAPISpec {
	t.Helper()
	specPath := filepath.Join(t.TempDir(), "openapi.yaml")
	spec := `
openapi: 3.0.3
info:
  title: Widget API
  version: 1.0.0
paths:
  /widget:
    get:
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                additionalProperties: false
                required: [id, name]
                properties:
                  id:
                    type: integer
                  name:
                    type: string
`
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatalf("writing spec: %v", err)
	}
	return supergo.MustOpenAPISpec(specPath)
}

func widgetHandler(payload map[string]any) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widget", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload) //nolint:errcheck
	})
	return mux
}

func TestFakeVerifySpecPasses(t *testing.T) {
	spy := &spyT{T: t}

	fake := supergo.NewFake(spy, widgetHandler(map[string]any{"id": 1, "name": "gear"})).
		VerifySpec(widgetSpec(t))

	resp, err := http.Get(fake.URL + "/widget")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if len(spy.errors) != 0 {
		t.Errorf("expected an on-spec response to pass, got errors: %v", spy.errors)
	}
}

// TestFakeVerifySpecFailsOnOffSpecResponse is the core differentiator: a fake
// whose response has drifted from the contract fails the test, something a
// hand-rolled fake would never catch.
func TestFakeVerifySpecFailsOnOffSpecResponse(t *testing.T) {
	spy := &spyT{T: t}

	// Drifted: missing the required "name", and carries an undeclared "color".
	fake := supergo.NewFake(spy, widgetHandler(map[string]any{"id": 1, "color": "red"})).
		VerifySpec(widgetSpec(t))

	resp, err := http.Get(fake.URL + "/widget")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if len(spy.errors) == 0 {
		t.Fatal("expected VerifySpec to fail on an off-spec response")
	}
	if !strings.Contains(strings.Join(spy.errors, ";"), "violated spec") {
		t.Errorf("unexpected error message: %v", spy.errors)
	}
}

func TestFakeVerifySpecFailsOnUndeclaredOperation(t *testing.T) {
	spy := &spyT{T: t}

	handler := http.NewServeMux()
	handler.HandleFunc("GET /unknown", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`)) //nolint:errcheck
	})

	fake := supergo.NewFake(spy, handler).VerifySpec(widgetSpec(t))

	resp, err := http.Get(fake.URL + "/unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if len(spy.errors) == 0 {
		t.Error("expected VerifySpec to fail when the fake serves an operation the spec does not declare")
	}
}
