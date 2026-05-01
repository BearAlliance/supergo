package supergo_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bearalliance/go-super/pkg/supergo"
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

	// Login: sets a session cookie.
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "tok123"})
		w.WriteHeader(http.StatusOK)
	})

	// Profile: requires session cookie.
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

func TestBasicStatusAssertion(t *testing.T) {
	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		Test(t)
}

func TestHeaderAssertion(t *testing.T) {
	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		ExpectHeader("Content-Type", "application/json").
		Test(t)
}

func TestHeaderExactAssertion(t *testing.T) {
	supergo.New(testMux()).
		Get("/echo-header").
		Set("X-Custom", "hello").
		Expect(200).
		ExpectHeaderExact("X-Echo", "hello").
		Test(t)
}

func TestExpectBodyJSONSubset(t *testing.T) {
	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		ExpectBody(`{"users":[{"name":"alice"}]}`).
		Test(t)
}

func TestExpectBodyExact(t *testing.T) {
	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		ExpectBodyExact(`{"users":[{"name":"alice"},{"name":"bob"}]}`).
		Test(t)
}

func TestExpectBodyContains(t *testing.T) {
	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		ExpectBodyContains("alice").
		Test(t)
}

func TestExpectBodyContainsJSON(t *testing.T) {
	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		ExpectBodyContainsJSON("users.0.name", "alice").
		ExpectBodyContainsJSON("users.1.name", "bob").
		Test(t)
}

func TestExpectBodyMatchesJSON(t *testing.T) {
	type User struct {
		Name string `json:"name"`
	}
	type Body struct {
		Users []User `json:"users"`
	}
	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		ExpectBodyMatchesJSON(Body{Users: []User{{Name: "alice"}, {Name: "bob"}}}).
		Test(t)
}

func TestSendJSON(t *testing.T) {
	supergo.New(testMux()).
		Post("/users").
		SendJSON(map[string]string{"name": "charlie"}).
		Expect(201).
		ExpectBodyContainsJSON("name", "charlie").
		Test(t)
}

func TestSendForm(t *testing.T) {
	v := url.Values{"name": {"dave"}}
	supergo.New(testMux()).
		Post("/form").
		SendForm(v).
		Expect(200).
		ExpectBodyContains("dave").
		Test(t)
}

func TestQueryParam(t *testing.T) {
	supergo.New(testMux()).
		Get("/search").
		Query("q", "golang").
		Expect(200).
		ExpectBodyContains("golang").
		Test(t)
}

func TestExpectFn(t *testing.T) {
	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		ExpectFn(func(res *supergo.Response) error {
			if !strings.Contains(string(res.Body), "alice") {
				return fmt.Errorf("expected alice in body")
			}
			return nil
		}).
		Test(t)
}

func TestDoIdempotent(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(200)
	})
	req := supergo.New(handler).Get("/")
	req.Do() //nolint:errcheck
	req.Do() //nolint:errcheck
	if callCount != 1 {
		t.Errorf("expected handler to be called once, got %d", callCount)
	}
}

func TestAgentCookiePersistence(t *testing.T) {
	agent := supergo.NewAgent(testMux())

	// Login sets a session cookie.
	agent.Post("/login").
		Expect(200).
		Test(t)

	// Profile requires the session cookie — agent should send it automatically.
	agent.Get("/profile").
		Expect(200).
		ExpectBodyContainsJSON("name", "alice").
		Test(t)
}

func TestAgentCookieDeletion(t *testing.T) {
	agent := supergo.NewAgent(testMux())

	agent.Post("/login").Expect(200).Test(t)

	// Logout deletes the cookie.
	agent.Delete("/logout").Expect(200).Test(t)

	// Profile should now be unauthorized.
	agent.Get("/profile").Expect(401).Test(t)
}

func TestAgentHistory(t *testing.T) {
	agent := supergo.NewAgent(testMux())

	agent.Post("/login").Expect(200).Test(t)
	agent.Get("/profile").Expect(200).Test(t)

	history := agent.History()
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if history[0].Method != "POST" || history[0].Path != "/login" {
		t.Errorf("unexpected first history entry: %s %s", history[0].Method, history[0].Path)
	}
	if history[1].Method != "GET" || history[1].Path != "/profile" {
		t.Errorf("unexpected second history entry: %s %s", history[1].Method, history[1].Path)
	}
	if history[0].Response.StatusCode != 200 {
		t.Errorf("expected first response status 200, got %d", history[0].Response.StatusCode)
	}
	if len(history[0].Assertions) == 0 {
		t.Error("expected assertions to be recorded in history")
	}
}

func TestAgentHistoryAssertionNames(t *testing.T) {
	agent := supergo.NewAgent(testMux())

	agent.Get("/users").
		Expect(200).
		ExpectHeader("Content-Type", "application/json").
		Test(t)

	history := agent.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	assertions := history[0].Assertions
	if len(assertions) != 2 {
		t.Fatalf("expected 2 assertion names, got %d: %v", len(assertions), assertions)
	}
	if assertions[0] != "status 200" {
		t.Errorf("expected first assertion name 'status 200', got %q", assertions[0])
	}
}

func TestAgentClearHistory(t *testing.T) {
	agent := supergo.NewAgent(testMux())
	agent.Get("/users").Expect(200).Test(t)
	agent.ClearHistory()
	if len(agent.History()) != 0 {
		t.Error("expected empty history after ClearHistory")
	}
}

func TestTestReturnsResponse(t *testing.T) {
	res := supergo.New(testMux()).Get("/users").Expect(200).Test(t)
	if res == nil {
		t.Fatal("expected non-nil response")
	}
	if res.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}
	if len(res.Body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestNewServer(t *testing.T) {
	server := &http.Server{Handler: testMux()}
	supergo.NewServer(server).
		Get("/users").
		Expect(200).
		Test(t)
}

// ── Stub ──────────────────────────────────────────────────────────────────────

func TestStubRespondJSON(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/ping").RespondJSON(200, map[string]string{"status": "ok"})

	resp, err := http.Get(stub.URL + "/ping")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected json content-type, got %s", ct)
	}
}

func TestStubCapturesRequest(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/search").RespondJSON(200, map[string]string{})

	http.Get(stub.URL + "/search?q=golang") //nolint:errcheck

	reqs := stub.Received("GET", "/search")
	if len(reqs) != 1 {
		t.Fatalf("expected 1 captured request, got %d", len(reqs))
	}
	if reqs[0].RawQuery != "q=golang" {
		t.Errorf("expected query q=golang, got %s", reqs[0].RawQuery)
	}
	if reqs[0].Path != "/search" {
		t.Errorf("expected path /search, got %s", reqs[0].Path)
	}
	if reqs[0].Method != "GET" {
		t.Errorf("expected method GET, got %s", reqs[0].Method)
	}
}

func TestStubCapturesMultipleCalls(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/ping").RespondJSON(200, nil)

	http.Get(stub.URL + "/ping") //nolint:errcheck
	http.Get(stub.URL + "/ping") //nolint:errcheck
	http.Get(stub.URL + "/ping") //nolint:errcheck

	if n := len(stub.Received("GET", "/ping")); n != 3 {
		t.Errorf("expected 3 captured requests, got %d", n)
	}
}

func TestStubReceivedReturnsEmptySliceForUnhitRoute(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/ping").RespondJSON(200, nil)

	reqs := stub.Received("GET", "/never-called")
	if reqs == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(reqs) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(reqs))
	}
}

func TestStubReceivedReturnsCopy(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/ping").RespondJSON(200, nil)

	http.Get(stub.URL + "/ping") //nolint:errcheck

	a := stub.Received("GET", "/ping")
	b := stub.Received("GET", "/ping")

	// Appending to one result must not affect the other.
	a = append(a, &supergo.CapturedRequest{})
	if len(b) != 1 {
		t.Errorf("Received should return independent copies, but b grew to %d", len(b))
	}
}

func TestStubCapturesRequestBody(t *testing.T) {
	stub := supergo.NewStub(t).
		On("POST", "/data").RespondJSON(201, nil)

	http.Post(stub.URL+"/data", "application/json", strings.NewReader(`{"x":1}`)) //nolint:errcheck

	reqs := stub.Received("POST", "/data")
	if len(reqs) != 1 {
		t.Fatalf("expected 1 captured request, got %d", len(reqs))
	}
	if !strings.Contains(string(reqs[0].Body), `"x"`) {
		t.Errorf("expected body to contain x, got %s", reqs[0].Body)
	}
}

func TestStubCapturesHeaders(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/headers").RespondJSON(200, nil)

	req, _ := http.NewRequest("GET", stub.URL+"/headers", nil)
	req.Header.Set("X-Request-ID", "abc123")
	http.DefaultClient.Do(req) //nolint:errcheck

	reqs := stub.Received("GET", "/headers")
	if len(reqs) != 1 {
		t.Fatalf("expected 1 captured request, got %d", len(reqs))
	}
	if reqs[0].Header.Get("X-Request-Id") != "abc123" {
		t.Errorf("expected X-Request-Id: abc123, got %s", reqs[0].Header.Get("X-Request-Id"))
	}
}

func TestStubRespond(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/empty").Respond(204, nil)

	resp, err := http.Get(stub.URL + "/empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestStubRespondJSONDynamic(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/echo").RespondJSON(200, func(r *http.Request) any {
			return map[string]string{"title": r.URL.Query().Get("title")}
		})

	resp, err := http.Get(stub.URL + "/echo?title=Dune")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Dune") {
		t.Errorf("expected body to contain Dune, got %s", body)
	}
}

func TestStubMustBeCalledPasses(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/ping").MustBeCalled().RespondJSON(200, nil)

	http.Get(stub.URL + "/ping") //nolint:errcheck
	// Cleanup runs after this test: no error expected.
}

func TestStubMustBeCalledFails(t *testing.T) {
	spy := &spyT{T: t}

	// Register our assertion first; t.Cleanup runs LIFO, so it fires after
	// MustBeCalled's cleanup and can observe the captured error.
	t.Cleanup(func() {
		if len(spy.errors) == 0 {
			t.Error("expected MustBeCalled to record an error for an uncalled route")
		}
	})

	supergo.NewStub(spy).
		On("GET", "/never").MustBeCalled().RespondJSON(200, nil)
	// Never hit the stub — MustBeCalled cleanup should fire an error into spy.
}

// spyT captures Errorf calls without failing the real test, allowing tests to
// assert that a piece of code under test *would* fail a test.
type spyT struct {
	*testing.T
	errors []string
}

func (s *spyT) Errorf(format string, args ...any) {
	s.errors = append(s.errors, fmt.Sprintf(format, args...))
}

func TestStubRespondFn(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/custom").RespondFn(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(202)
			w.Write([]byte("custom")) //nolint:errcheck
		})

	resp, err := http.Get(stub.URL + "/custom")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Errorf("expected 202, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "custom" {
		t.Errorf("expected body 'custom', got %s", body)
	}
}
