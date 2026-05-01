// Package supergo provides a supertest-inspired HTTP testing helper for Go.
//
// It wraps an [http.Handler] and exposes a fluent, chainable API for building
// requests and asserting on responses, all in-process via [net/http/httptest],
// with no real TCP ports or cleanup needed.
//
// # One-shot requests
//
//	supergo.New(myHandler).
//	    Get("/users").
//	    Set("Accept", "application/json").
//	    Expect(200).
//	    ExpectHeader("Content-Type", "application/json").
//	    ExpectBody(`{"users":[]}`).
//	    Test(t)
//
// # Stateful agent (persists cookies, records history)
//
//	agent := supergo.NewAgent(myHandler)
//
//	agent.Post("/login").
//	    SendJSON(map[string]string{"user": "alice", "pass": "secret"}).
//	    Expect(200).
//	    Test(t)
//
//	agent.Get("/profile").
//	    Expect(200).
//	    ExpectBodyContainsJSON("name", "alice").
//	    Test(t)
//
//	history := agent.History()
package supergo

import "net/http"

// New returns a [Requester] bound to handler. Call one of the HTTP-method
// starters (Get, Post, etc.) on the returned value to build a request.
func New(handler http.Handler) *Requester {
	return &Requester{handler: handler}
}

// NewServer extracts the Handler from server and returns a [Requester] bound
// to it. This is a convenience for callers who construct an *http.Server.
func NewServer(server *http.Server) *Requester {
	return &Requester{handler: server.Handler}
}

// NewAgent returns a stateful [Agent] that persists cookies across requests
// and records a history of all request/response cycles.
func NewAgent(handler http.Handler) *Agent {
	return newAgent(handler)
}
