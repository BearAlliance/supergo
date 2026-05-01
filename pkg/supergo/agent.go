package supergo

import (
	"net/http"
	"sync"
	"time"
)

// Agent is a stateful HTTP test client that persists cookies across requests
// and records a history of all request/response cycles.
type Agent struct {
	handler http.Handler
	jar     *cookieJar
	history []*HistoryEntry
	mu      sync.Mutex
}

func newAgent(handler http.Handler) *Agent {
	return &Agent{
		handler: handler,
		jar:     newCookieJar(),
	}
}

func (a *Agent) newRequest(method, path string) *Request {
	return &Request{
		handler:     a.handler,
		method:      method,
		path:        path,
		header:      make(http.Header),
		queryParams: make(map[string][]string),
		agent:       a,
	}
}

// Get starts a GET request chain.
func (a *Agent) Get(path string) *Request { return a.newRequest("GET", path) }

// Post starts a POST request chain.
func (a *Agent) Post(path string) *Request { return a.newRequest("POST", path) }

// Put starts a PUT request chain.
func (a *Agent) Put(path string) *Request { return a.newRequest("PUT", path) }

// Patch starts a PATCH request chain.
func (a *Agent) Patch(path string) *Request { return a.newRequest("PATCH", path) }

// Delete starts a DELETE request chain.
func (a *Agent) Delete(path string) *Request { return a.newRequest("DELETE", path) }

// Head starts a HEAD request chain.
func (a *Agent) Head(path string) *Request { return a.newRequest("HEAD", path) }

// Options starts an OPTIONS request chain.
func (a *Agent) Options(path string) *Request { return a.newRequest("OPTIONS", path) }

// History returns a copy of the recorded request/response history.
func (a *Agent) History() []*HistoryEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := make([]*HistoryEntry, len(a.history))
	copy(cp, a.history)
	return cp
}

// ClearHistory discards all recorded history entries.
func (a *Agent) ClearHistory() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.history = nil
}

func (a *Agent) record(r *Request, res *Response) {
	entry := &HistoryEntry{
		Method:     r.method,
		Path:       r.path,
		ReqHeader:  r.header.Clone(),
		Response:   res,
		Assertions: append([]string(nil), r.assertionNames...),
		ExecutedAt: time.Now(),
	}
	a.mu.Lock()
	a.history = append(a.history, entry)
	a.mu.Unlock()
}

// cookieJar is a minimal name-keyed in-memory cookie store for test use.
type cookieJar struct {
	mu      sync.Mutex
	cookies map[string]*http.Cookie
}

func newCookieJar() *cookieJar {
	return &cookieJar{cookies: make(map[string]*http.Cookie)}
}

// setOn copies all stored cookies onto the outbound request.
func (j *cookieJar) setOn(req *http.Request) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, c := range j.cookies {
		req.AddCookie(c)
	}
}

// harvest absorbs cookies from a response, respecting MaxAge-based deletions.
func (j *cookieJar) harvest(cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, c := range cookies {
		if c.MaxAge < 0 {
			delete(j.cookies, c.Name)
		} else {
			j.cookies[c.Name] = c
		}
	}
}
