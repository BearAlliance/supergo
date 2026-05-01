package supergo_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bearalliance/supergo/pkg/supergo"
)

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
	type user struct {
		Name string `json:"name"`
	}
	type body struct {
		Users []user `json:"users"`
	}

	supergo.New(testMux()).
		Get("/users").
		Expect(200).
		ExpectBodyMatchesJSON(body{Users: []user{{Name: "alice"}, {Name: "bob"}}}).
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
