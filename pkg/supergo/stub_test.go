package supergo_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bearalliance/supergo/pkg/supergo"
)

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

	_ = append(a, &supergo.CapturedRequest{})
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

func TestStubStrictFailsOnUnexpectedRequest(t *testing.T) {
	spy := &spyT{T: t}
	stub := supergo.NewStub(spy).Strict()

	http.Get(stub.URL + "/unregistered") //nolint:errcheck

	if len(spy.errors) == 0 {
		t.Error("expected Strict to record an error for an unregistered route")
	}
}

func TestStubStrictPassesForRegisteredRoute(t *testing.T) {
	spy := &spyT{T: t}
	stub := supergo.NewStub(spy).Strict().
		On("GET", "/ping").RespondJSON(200, nil)

	http.Get(stub.URL + "/ping") //nolint:errcheck

	if len(spy.errors) != 0 {
		t.Errorf("expected no errors for a registered route, got: %v", spy.errors)
	}
}

func TestStubMustBeCalledPasses(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/ping").MustBeCalled().RespondJSON(200, nil)

	http.Get(stub.URL + "/ping") //nolint:errcheck
}

func TestStubMustBeCalledFails(t *testing.T) {
	spy := &spyT{T: t}

	t.Cleanup(func() {
		if len(spy.errors) == 0 {
			t.Error("expected MustBeCalled to record an error for an uncalled route")
		}
	})

	supergo.NewStub(spy).
		On("GET", "/never").MustBeCalled().RespondJSON(200, nil)
}

func TestStubMustAllBeCalledPasses(t *testing.T) {
	stub := supergo.NewStub(t).
		MustAllBeCalled().
		On("GET", "/ping").RespondJSON(200, nil).
		On("POST", "/pong").RespondJSON(201, nil)

	http.Get(stub.URL + "/ping") //nolint:errcheck
	http.Post(stub.URL+"/pong", "application/json", nil) //nolint:errcheck
}

func TestStubMustAllBeCalledFails(t *testing.T) {
	spy := &spyT{T: t}

	t.Cleanup(func() {
		if len(spy.errors) == 0 {
			t.Error("expected MustAllBeCalled to record an error for an uncalled route")
			return
		}
		if !strings.Contains(spy.errors[0], "supergo: stub route POST /pong was never called") {
			t.Fatalf("unexpected MustAllBeCalled error: %v", spy.errors)
		}
	})

	stub := supergo.NewStub(spy).
		MustAllBeCalled().
		On("GET", "/ping").RespondJSON(200, nil).
		On("POST", "/pong").RespondJSON(201, nil)

	http.Get(stub.URL + "/ping") //nolint:errcheck
}

func TestStubCapturedRequestQuery(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/search").RespondJSON(200, nil)

	http.Get(stub.URL + "/search?foo=bar&baz=qux") //nolint:errcheck

	reqs := stub.Received("GET", "/search")
	if reqs[0].Query().Get("foo") != "bar" {
		t.Errorf("expected foo=bar, got %s", reqs[0].Query().Get("foo"))
	}
	if reqs[0].Query().Get("baz") != "qux" {
		t.Errorf("expected baz=qux, got %s", reqs[0].Query().Get("baz"))
	}
}

func TestStubSequenceTwoCalls(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/step").
		RespondJSON(200, map[string]string{"step": "first"}).
		ThenRespondJSON(200, map[string]string{"step": "second"})

	body := func() string {
		resp, err := http.Get(stub.URL + "/step")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	if first := body(); !strings.Contains(first, "first") {
		t.Errorf("expected first response to contain 'first', got: %s", first)
	}
	if second := body(); !strings.Contains(second, "second") {
		t.Errorf("expected second response to contain 'second', got: %s", second)
	}
}

func TestStubSequenceRepeatsLast(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/step").
		RespondJSON(200, map[string]string{"step": "first"}).
		ThenRespondJSON(200, map[string]string{"step": "last"})

	get := func() string {
		resp, err := http.Get(stub.URL + "/step")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	get()
	if second := get(); !strings.Contains(second, "last") {
		t.Errorf("expected second response to contain 'last', got: %s", second)
	}
	if third := get(); !strings.Contains(third, "last") {
		t.Errorf("expected third response to repeat last, got: %s", third)
	}
}

func TestStubSequenceThenRespondFn(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/step").
		RespondJSON(200, map[string]string{"step": "first"}).
		ThenRespondFn(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(202)
			w.Write([]byte("custom")) //nolint:errcheck
		})

	http.Get(stub.URL + "/step") //nolint:errcheck

	resp, err := http.Get(stub.URL + "/step")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Errorf("expected 202 on second call, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "custom" {
		t.Errorf("expected body 'custom', got %s", body)
	}
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
