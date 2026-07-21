package supergo

import (
	"sync"
	"testing"
)

// callLog is the shared request-capture and interaction-guard machinery used by
// both Stub and Fake. It records every request a test double receives, exposes
// it through Received, and registers t.Cleanup guards that assert on call counts.
//
// noun is woven into guard failure messages ("stub" or "fake") so each double
// reports failures in its own terms.
type callLog struct {
	t          testing.TB
	noun       string
	mu         sync.Mutex
	calls      map[string][]*CapturedRequest // key: "METHOD /path"
	registered []string
}

func newCallLog(t testing.TB, noun string) *callLog {
	return &callLog{
		t:     t,
		noun:  noun,
		calls: make(map[string][]*CapturedRequest),
	}
}

// record stores one captured request under the "METHOD /path" key.
func (c *callLog) record(method, path string, req *CapturedRequest) {
	key := method + " " + path
	c.mu.Lock()
	c.calls[key] = append(c.calls[key], req)
	c.mu.Unlock()
}

// markRegistered notes a "METHOD /path" pattern as one this double knows about,
// so MustAllBeCalled can iterate the full set later.
func (c *callLog) markRegistered(pattern string) {
	c.mu.Lock()
	if !containsString(c.registered, pattern) {
		c.registered = append(c.registered, pattern)
	}
	c.mu.Unlock()
}

// Received returns all requests captured for the given method and path, in
// arrival order. Returns a non-nil empty slice if the route was never hit.
func (c *callLog) Received(method, path string) []*CapturedRequest {
	return c.ReceivedParts(method + " " + path)
}

// ReceivedParts is like Received but takes a preassembled "METHOD /path" key.
func (c *callLog) ReceivedParts(key string) []*CapturedRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	got := c.calls[key]
	cp := make([]*CapturedRequest, len(got))
	copy(cp, got)
	return cp
}

// mustBeCalled registers a cleanup assertion that fails the test if the route
// receives no requests by the time the test ends.
func (c *callLog) mustBeCalled(method, path string) {
	c.t.Cleanup(func() {
		if len(c.Received(method, path)) == 0 {
			c.t.Errorf("supergo: %s route %s %s was never called", c.noun, method, path)
		}
	})
}

// mustBeCalledTimes registers a cleanup assertion that fails the test if the
// route is not called exactly n times by the time the test ends.
func (c *callLog) mustBeCalledTimes(method, path string, n int) {
	c.t.Cleanup(func() {
		got := len(c.Received(method, path))
		if got != n {
			c.t.Errorf("supergo: %s route %s %s: expected %d call(s), got %d", c.noun, method, path, n, got)
		}
	})
}

// mustAllBeCalled registers a cleanup assertion that fails the test if any
// registered route received no requests by the time the test ends.
func (c *callLog) mustAllBeCalled() {
	c.t.Cleanup(func() {
		c.mu.Lock()
		routes := append([]string(nil), c.registered...)
		c.mu.Unlock()
		for _, route := range routes {
			if len(c.ReceivedParts(route)) == 0 {
				c.t.Errorf("supergo: %s route %s was never called", c.noun, route)
			}
		}
	})
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
