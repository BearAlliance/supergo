package supergo

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// CapturedRequest is an immutable snapshot of one HTTP request received by a Stub.
// All fields are safe to read from any goroutine after the request completes.
type CapturedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// Query parses RawQuery and returns the query parameters as url.Values.
func (c *CapturedRequest) Query() url.Values {
	v, _ := url.ParseQuery(c.RawQuery)
	return v
}

// Stub is a real TCP HTTP server used to stand in for external services in tests.
// Create one with NewStub; its URL field is ready to pass to the system under test.
//
// All On calls must be made before stub.URL is passed to the system under test.
// Unregistered paths return 404 by default; call Strict() to fail the test instead.
type Stub struct {
	// URL is the base URL of the stub server (e.g. "http://127.0.0.1:PORT"),
	// with no trailing slash. Pass it to the system under test.
	URL        string
	t          testing.TB
	server     *httptest.Server
	mux        *http.ServeMux
	mu         sync.Mutex
	calls      map[string][]*CapturedRequest // key: "METHOD /path"
	registered []string
}

// NewStub creates a stub HTTP server. The server is closed automatically via
// t.Cleanup, so callers do not need to close it themselves.
func NewStub(t testing.TB) *Stub {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	s := &Stub{
		URL:    server.URL,
		t:      t,
		server: server,
		mux:    mux,
		calls:  make(map[string][]*CapturedRequest),
	}
	t.Cleanup(server.Close)
	return s
}

// Strict enables strict mode: any request that does not match a registered On
// route immediately fails the test. Call it before On:
//
//	supergo.NewStub(t).Strict().On("GET", "/cover").RespondJSON(200, ...)
func (s *Stub) Strict() *Stub {
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.t.Errorf("supergo: unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	return s
}

// On registers a handler for the given method and path, returning a *StubRoute
// for response configuration. method must be uppercase (e.g. "GET") and path
// must start with "/" (e.g. "/cover").
func (s *Stub) On(method, path string) *StubRoute {
	return &StubRoute{stub: s, method: method, path: path}
}

// MustAllBeCalled registers a cleanup assertion that fails the test if any
// registered stub route receives no requests by the time the test ends.
//
// Call it once on the stub when every route you register for that stub should
// be exercised:
//
//	supergo.NewStub(t).MustAllBeCalled().On("GET", "/cover").RespondJSON(200, ...)
func (s *Stub) MustAllBeCalled() *Stub {
	s.t.Cleanup(func() {
		s.mu.Lock()
		routes := append([]string(nil), s.registered...)
		s.mu.Unlock()
		for _, route := range routes {
			if len(s.ReceivedParts(route)) == 0 {
				s.t.Errorf("supergo: stub route %s was never called", route)
			}
		}
	})
	return s
}

// Received returns all requests captured for the given method and path, in
// arrival order. Returns a non-nil empty slice if the route was never hit.
func (s *Stub) Received(method, path string) []*CapturedRequest {
	return s.ReceivedParts(method + " " + path)
}

func (s *Stub) ReceivedParts(key string) []*CapturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	got := s.calls[key]
	cp := make([]*CapturedRequest, len(got))
	copy(cp, got)
	return cp
}

func (s *Stub) registerSequence(method, path string, seq *StubSequence) {
	pattern := method + " " + path
	s.mu.Lock()
	if !containsString(s.registered, pattern) {
		s.registered = append(s.registered, pattern)
	}
	s.mu.Unlock()
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()

		cap := &CapturedRequest{
			Method:   r.Method,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
			Header:   r.Header.Clone(),
			Body:     bodyBytes,
		}
		s.mu.Lock()
		s.calls[pattern] = append(s.calls[pattern], cap)
		s.mu.Unlock()

		seq.mu.Lock()
		idx := seq.callIndex
		if idx < len(seq.handlers)-1 {
			seq.callIndex++
		}
		fn := seq.handlers[idx]
		seq.mu.Unlock()

		// Restore body so RespondFn handlers can read it if needed.
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		fn(w, r)
	})
}

// StubRoute configures the response for one method+path combination on a Stub.
type StubRoute struct {
	stub   *Stub
	method string
	path   string
}

// MustBeCalled registers a cleanup assertion that fails the test if this route
// receives no requests by the time the test ends. Chain it before the terminal
// Respond* call:
//
//	stub.On("GET", "/cover").MustBeCalled().RespondJSON(200, ...)
func (sr *StubRoute) MustBeCalled() *StubRoute {
	sr.stub.t.Cleanup(func() {
		if len(sr.stub.Received(sr.method, sr.path)) == 0 {
			sr.stub.t.Errorf("supergo: stub route %s %s was never called", sr.method, sr.path)
		}
	})
	return sr
}

// MustBeCalledTimes registers a cleanup assertion that fails the test if this
// route is not called exactly n times by the time the test ends. Chain it
// before the terminal Respond* call:
//
//	stub.On("GET", "/cover").MustBeCalledTimes(3).RespondJSON(200, ...)
func (sr *StubRoute) MustBeCalledTimes(n int) *StubRoute {
	sr.stub.t.Cleanup(func() {
		got := len(sr.stub.Received(sr.method, sr.path))
		if got != n {
			sr.stub.t.Errorf("supergo: stub route %s %s: expected %d call(s), got %d", sr.method, sr.path, n, got)
		}
	})
	return sr
}

// Respond registers a fixed-status, fixed-body response for this route.
// body may be nil to produce an empty response body.
func (sr *StubRoute) Respond(status int, body []byte) *StubSequence {
	return sr.RespondFn(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if len(body) > 0 {
			w.Write(body) //nolint:errcheck
		}
	})
}

// RespondJSON registers a response that sets Content-Type: application/json
// and encodes v as JSON. v may be either:
//
//   - a static value, marshalled once at registration time (panics immediately
//     if the value cannot be encoded), or
//   - a func(*http.Request) any, called on every request so the response data
//     can be derived from the incoming request (e.g. echoing a query parameter).
func (sr *StubRoute) RespondJSON(status int, v any) *StubSequence {
	return sr.RespondFn(makeJSONHandler(status, v))
}

// RespondFn registers a raw HandlerFunc for this route, giving full control
// over headers, status, and body. The stub's capture logic still runs before fn.
func (sr *StubRoute) RespondFn(fn http.HandlerFunc) *StubSequence {
	seq := &StubSequence{Stub: sr.stub, handlers: []http.HandlerFunc{fn}}
	sr.stub.registerSequence(sr.method, sr.path, seq)
	return seq
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// StubSequence is returned by StubRoute terminal methods. It exposes Then*
// methods for building ordered response sequences and embeds *Stub so URL,
// On, Received, and Strict are all accessible directly.
type StubSequence struct {
	*Stub
	mu        sync.Mutex
	handlers  []http.HandlerFunc
	callIndex int
}

// ThenRespond appends a fixed-status, fixed-body response to the sequence.
func (ss *StubSequence) ThenRespond(status int, body []byte) *StubSequence {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.handlers = append(ss.handlers, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if len(body) > 0 {
			w.Write(body) //nolint:errcheck
		}
	})
	return ss
}

// ThenRespondJSON appends a JSON response to the sequence. v follows the same
// static/dynamic rules as RespondJSON.
func (ss *StubSequence) ThenRespondJSON(status int, v any) *StubSequence {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.handlers = append(ss.handlers, makeJSONHandler(status, v))
	return ss
}

// ThenRespondFn appends a raw HandlerFunc to the sequence.
func (ss *StubSequence) ThenRespondFn(fn http.HandlerFunc) *StubSequence {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.handlers = append(ss.handlers, fn)
	return ss
}

// makeJSONHandler builds a handler that responds with JSON. v may be a static
// value (marshalled once) or a func(*http.Request) any (called per request).
func makeJSONHandler(status int, v any) http.HandlerFunc {
	if fn, ok := v.(func(*http.Request) any); ok {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(fn(r)) //nolint:errcheck
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		panic("supergo: RespondJSON could not encode value: " + err.Error())
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(b) //nolint:errcheck
	}
}
