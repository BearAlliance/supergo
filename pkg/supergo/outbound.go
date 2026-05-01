package supergo

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// NewOutboundHTTPClient returns an HTTP client that only allows requests to the
// provided base URLs. This is useful in tests that should fail if the system
// under test calls an unexpected external service.
//
// Pass stub URLs for services you mock with NewStub. You may also pass known
// real service base URLs that the test intentionally allows, such as a shared
// cache or database proxy.
func NewOutboundHTTPClient(t testing.TB, allowedBaseURLs ...string) *http.Client {
	t.Helper()

	allowedHosts := make(map[string]struct{}, len(allowedBaseURLs))
	for _, rawURL := range allowedBaseURLs {
		if rawURL == "" {
			continue
		}
		u, err := url.Parse(rawURL)
		if err != nil || u.Host == "" {
			t.Fatalf("supergo: invalid allowed outbound URL %q", rawURL)
		}
		allowedHosts[u.Host] = struct{}{}
	}

	base := http.DefaultTransport
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		base = transport.Clone()
	}

	return &http.Client{
		Transport: &outboundGuardTransport{
			t:            t,
			allowedHosts: allowedHosts,
			next:         base,
		},
	}
}

type outboundGuardTransport struct {
	t            testing.TB
	allowedHosts map[string]struct{}
	next         http.RoundTripper
}

func (tr *outboundGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, ok := tr.allowedHosts[req.URL.Host]; !ok {
		tr.t.Helper()
		tr.t.Errorf("supergo: unexpected outbound request to %s", req.URL.String())
		return nil, fmt.Errorf("supergo: unexpected outbound request to %s", req.URL.String())
	}
	return tr.next.RoundTrip(req)
}
