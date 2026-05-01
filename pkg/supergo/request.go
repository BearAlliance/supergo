package supergo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// assertionFn is an internal assertion closure resolved in Test().
type assertionFn func(r *Response, t testing.TB)

// Request is the central builder and assertion chain. Create one via New(),
// NewAgent(), or the HTTP-method starters on Requester or Agent.
type Request struct {
	handler        http.Handler
	method         string
	path           string
	header         http.Header
	body           io.Reader
	queryParams    url.Values
	assertions     []assertionFn
	assertionNames []string
	agent          *Agent
	response       *Response
	executed       bool
}

// Set adds a request header.
func (r *Request) Set(key, value string) *Request {
	r.header.Set(key, value)
	return r
}

// Auth sets HTTP Basic Auth credentials.
func (r *Request) Auth(username, password string) *Request {
	req, _ := http.NewRequest("GET", "/", nil)
	req.SetBasicAuth(username, password)
	r.header.Set("Authorization", req.Header.Get("Authorization"))
	return r
}

// Send sets the request body. If body is a string or []byte it is used as-is.
// Any other type is JSON-encoded and Content-Type is set to application/json.
func (r *Request) Send(body interface{}) *Request {
	switch v := body.(type) {
	case string:
		r.body = strings.NewReader(v)
	case []byte:
		r.body = bytes.NewReader(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			panic(fmt.Sprintf("supergo: Send could not JSON-encode body: %v", err))
		}
		r.body = bytes.NewReader(b)
		if r.header.Get("Content-Type") == "" {
			r.header.Set("Content-Type", "application/json")
		}
	}
	return r
}

// SendJSON JSON-encodes v and sets Content-Type: application/json.
func (r *Request) SendJSON(v interface{}) *Request {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("supergo: SendJSON could not JSON-encode body: %v", err))
	}
	r.body = bytes.NewReader(b)
	r.header.Set("Content-Type", "application/json")
	return r
}

// SendForm sets a URL-encoded form body and Content-Type: application/x-www-form-urlencoded.
func (r *Request) SendForm(values url.Values) *Request {
	r.body = strings.NewReader(values.Encode())
	r.header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

// Query appends a query parameter to the request URL.
func (r *Request) Query(key, value string) *Request {
	r.queryParams.Add(key, value)
	return r
}

// Do executes the request against the handler using httptest.NewRecorder.
// It is idempotent: subsequent calls return the cached response.
func (r *Request) Do() (*Response, error) {
	if r.executed {
		return r.response, nil
	}
	r.executed = true

	// Build URL with query params.
	target := r.path
	if len(r.queryParams) > 0 {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target = target + sep + r.queryParams.Encode()
	}

	req, err := http.NewRequest(r.method, target, r.body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	for k, vs := range r.header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	if r.agent != nil {
		r.agent.jar.setOn(req)
	}

	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	result := rec.Result()

	if r.agent != nil {
		r.agent.jar.harvest(result.Cookies())
	}

	bodyBytes, err := io.ReadAll(result.Body)
	result.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	r.response = &Response{
		StatusCode: result.StatusCode,
		Header:     result.Header,
		Body:       bodyBytes,
	}
	return r.response, nil
}
