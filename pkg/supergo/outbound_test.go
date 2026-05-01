package supergo_test

import (
	"testing"

	"github.com/bearalliance/supergo/pkg/supergo"
)

func TestNewOutboundHTTPClientAllowsConfiguredStubURL(t *testing.T) {
	stub := supergo.NewStub(t).
		On("GET", "/ping").RespondJSON(200, map[string]string{"status": "ok"})

	client := supergo.NewOutboundHTTPClient(t, stub.URL)

	resp, err := client.Get(stub.URL + "/ping")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNewOutboundHTTPClientRejectsUnexpectedHost(t *testing.T) {
	spy := &spyT{T: t}
	client := supergo.NewOutboundHTTPClient(spy, "http://allowed.example")

	_, err := client.Get("http://blocked.example/ping")
	if err == nil {
		t.Fatal("expected error for unexpected outbound host")
	}
	if len(spy.errors) == 0 {
		t.Fatal("expected unexpected host to be reported as a test error")
	}
}
