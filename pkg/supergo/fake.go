package supergo

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Fake is a real TCP HTTP server that stands in for an external service using a
// working in-memory implementation you supply, rather than the canned responses
// of a Stub. Create one with NewFake and pass its URL to the system under test.
//
// A Fake is the next rung up from a Stub in the test-double taxonomy: where a
// Stub returns preprogrammed responses per method+path, a Fake runs an
// http.Handler with real behavior, so a POST mutates state that a later GET
// reflects. You could host such a handler yourself with httptest; the reason to
// route it through supergo is what Fake wraps around the behavior:
//
//   - VerifySpec validates every response the fake emits against an OpenAPI
//     spec on every call, so the fake cannot silently drift from the real
//     service's contract; and
//   - the same request-capture and interaction guards as Stub (Received,
//     MustBeCalled, MustBeCalledTimes) come for free.
//
// The server is closed automatically via t.Cleanup.
type Fake struct {
	// URL is the base URL of the fake server (e.g. "http://127.0.0.1:PORT"),
	// with no trailing slash. Pass it to the system under test.
	URL    string
	server *httptest.Server
	spec   *OpenAPISpec
	*callLog
}

// NewFake creates a fake HTTP server backed by handler, which supplies the
// fake's in-memory behavior. handler is ordinary Go, e.g. an http.ServeMux
// closing over an in-memory store; supergo does not generate behavior, it wraps
// it with request capture and optional spec verification. The server is closed
// automatically via t.Cleanup.
func NewFake(t testing.TB, handler http.Handler) *Fake {
	f := &Fake{callLog: newCallLog(t, "fake")}
	f.server = httptest.NewServer(f.wrap(handler))
	f.URL = f.server.URL
	t.Cleanup(f.server.Close)
	return f
}

// VerifySpec turns on OpenAPI conformance checking: every response the fake
// returns is validated against spec for the request's method and path, and the
// test fails if it does not match the declared operation, status, content type,
// or body schema. This guards against the fake drifting away from the real
// service's contract. It also fails requests to operations the spec does not
// declare, since there is no operation to validate against.
//
// Load the spec once with MustOpenAPISpec or LoadOpenAPISpec and reuse it.
func (f *Fake) VerifySpec(spec *OpenAPISpec) *Fake {
	f.spec = spec
	return f
}

// MustBeCalled registers a cleanup assertion that fails the test if the given
// method and path receives no requests by the time the test ends. path is
// matched against the concrete request path (e.g. "/books/1"), not a template.
func (f *Fake) MustBeCalled(method, path string) *Fake {
	f.mustBeCalled(method, path)
	return f
}

// MustBeCalledTimes registers a cleanup assertion that fails the test if the
// given method and path is not called exactly n times by the time the test
// ends.
func (f *Fake) MustBeCalledTimes(method, path string, n int) *Fake {
	f.mustBeCalledTimes(method, path, n)
	return f
}

// wrap decorates the caller's handler with request capture and, when a spec is
// set, response verification. It records the request, runs the handler against
// an in-memory recorder, optionally validates the recorded response, then
// forwards that response to the real client.
func (f *Fake) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		f.record(r.Method, r.URL.Path, &CapturedRequest{
			Method:   r.Method,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
			Header:   r.Header.Clone(),
			Body:     body,
		})

		// Restore the body so the handler can read it.
		r.Body = io.NopCloser(bytes.NewReader(body))

		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, r)

		if f.spec != nil {
			res := &Response{
				StatusCode: rec.Code,
				Header:     rec.Header().Clone(),
				Body:       rec.Body.Bytes(),
			}
			if err := f.spec.validateResponse(r.Method, r.URL.Path, res); err != nil {
				f.t.Errorf("supergo: fake response for %s %s violated spec: %v", r.Method, r.URL.Path, err)
			}
		}

		for key, values := range rec.Header() {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(rec.Code)
		w.Write(rec.Body.Bytes()) //nolint:errcheck
	})
}
