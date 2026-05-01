package supergo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Expect asserts that the response status code equals the given value.
func (r *Request) Expect(status int) *Request {
	r.assertionNames = append(r.assertionNames, fmt.Sprintf("status %d", status))
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		if res.StatusCode != status {
			t.Errorf("expected status %d, got %d", status, res.StatusCode)
		}
	})
	return r
}

// ExpectHeader asserts that the response contains a header whose canonical key
// matches key and whose value contains the given substring (case-insensitive key lookup).
func (r *Request) ExpectHeader(key, value string) *Request {
	r.assertionNames = append(r.assertionNames, fmt.Sprintf("header %s contains %q", key, value))
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		canonical := http.CanonicalHeaderKey(key)
		got := res.Header.Get(canonical)
		if !strings.Contains(got, value) {
			t.Errorf("expected header %s to contain %q, got %q", canonical, value, got)
		}
	})
	return r
}

// ExpectHeaderExact asserts that the response header equals value exactly.
func (r *Request) ExpectHeaderExact(key, value string) *Request {
	r.assertionNames = append(r.assertionNames, fmt.Sprintf("header %s == %q", key, value))
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		canonical := http.CanonicalHeaderKey(key)
		got := res.Header.Get(canonical)
		if got != value {
			t.Errorf("expected header %s to equal %q, got %q", canonical, value, got)
		}
	})
	return r
}

// ExpectBody asserts the response body. If expected is valid JSON it performs a
// subset (contains) check; otherwise it does an exact trimmed-string match.
func (r *Request) ExpectBody(expected string) *Request {
	var matcher bodyMatcher
	var desc string
	var parsed interface{}
	if json.Unmarshal([]byte(expected), &parsed) == nil {
		matcher = jsonContainsMatcher{expected: parsed}
		desc = "body JSON contains " + expected
	} else {
		matcher = exactMatcher{expected: expected}
		desc = "body exact match"
	}
	r.assertionNames = append(r.assertionNames, desc)
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		if err := matcher.match(res.Body); err != nil {
			t.Errorf("%v", err)
		}
	})
	return r
}

// ExpectBodyExact asserts an exact trimmed-string match of the response body.
func (r *Request) ExpectBodyExact(expected string) *Request {
	r.assertionNames = append(r.assertionNames, "body exact match")
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		if err := (exactMatcher{expected: expected}).match(res.Body); err != nil {
			t.Errorf("%v", err)
		}
	})
	return r
}

// ExpectBodyContains asserts that the response body contains the given substring.
func (r *Request) ExpectBodyContains(substr string) *Request {
	r.assertionNames = append(r.assertionNames, fmt.Sprintf("body contains %q", substr))
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		if err := (containsMatcher{substr: substr}).match(res.Body); err != nil {
			t.Errorf("%v", err)
		}
	})
	return r
}

// ExpectBodyMatchesJSON asserts that the response body JSON deep-equals v.
func (r *Request) ExpectBodyMatchesJSON(v interface{}) *Request {
	r.assertionNames = append(r.assertionNames, "body JSON deep equal")
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		if err := (jsonDeepEqualMatcher{expected: v}).match(res.Body); err != nil {
			t.Errorf("%v", err)
		}
	})
	return r
}

// ExpectBodyContainsJSON traverses the response body JSON using dot-path notation
// (e.g. "users.0.name") and asserts that the value at that path equals expected.
func (r *Request) ExpectBodyContainsJSON(path string, expected interface{}) *Request {
	r.assertionNames = append(r.assertionNames, fmt.Sprintf("body JSON path %q", path))
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		var root interface{}
		if err := json.Unmarshal(res.Body, &root); err != nil {
			t.Errorf("response body is not valid JSON: %v\nbody: %s", err, string(res.Body))
			return
		}
		got, err := dotPathGet(root, path)
		if err != nil {
			t.Errorf("JSON path %q: %v", path, err)
			return
		}
		if err := jsonContains(expected, got); err != nil {
			t.Errorf("JSON path %q: %v", path, err)
		}
	})
	return r
}

// ExpectFn adds a custom assertion function. Return a non-nil error to fail.
func (r *Request) ExpectFn(fn func(res *Response) error) *Request {
	r.assertionNames = append(r.assertionNames, "custom assertion")
	r.assertions = append(r.assertions, func(res *Response, t testing.TB) {
		t.Helper()
		if err := fn(res); err != nil {
			t.Errorf("assertion failed: %v", err)
		}
	})
	return r
}

// Test executes the request (if not already done), runs all queued assertions
// using t.Errorf so every assertion is evaluated, records history on the agent
// (if any), and returns the Response for further inspection.
func (r *Request) Test(t testing.TB) *Response {
	t.Helper()
	res, err := r.Do()
	if err != nil {
		t.Fatalf("supergo: request failed: %v", err)
		return nil
	}
	for _, fn := range r.assertions {
		fn(res, t)
	}
	if r.agent != nil {
		r.agent.record(r, res)
	}
	return res
}
