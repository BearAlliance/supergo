package supergo

import "net/http"

// Requester is returned by New(). Its sole purpose is to select the HTTP
// method, returning a *Request builder ready for further configuration.
type Requester struct {
	handler http.Handler
	agent   *Agent // nil for one-shot requests
}

func (r *Requester) newRequest(method, path string) *Request {
	return &Request{
		handler:     r.handler,
		method:      method,
		path:        path,
		header:      make(http.Header),
		queryParams: make(map[string][]string),
		agent:       r.agent,
	}
}

// Get starts a GET request chain.
func (r *Requester) Get(path string) *Request { return r.newRequest("GET", path) }

// Post starts a POST request chain.
func (r *Requester) Post(path string) *Request { return r.newRequest("POST", path) }

// Put starts a PUT request chain.
func (r *Requester) Put(path string) *Request { return r.newRequest("PUT", path) }

// Patch starts a PATCH request chain.
func (r *Requester) Patch(path string) *Request { return r.newRequest("PATCH", path) }

// Delete starts a DELETE request chain.
func (r *Requester) Delete(path string) *Request { return r.newRequest("DELETE", path) }

// Head starts a HEAD request chain.
func (r *Requester) Head(path string) *Request { return r.newRequest("HEAD", path) }

// Options starts an OPTIONS request chain.
func (r *Requester) Options(path string) *Request { return r.newRequest("OPTIONS", path) }
