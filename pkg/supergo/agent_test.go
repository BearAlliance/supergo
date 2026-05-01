package supergo_test

import (
	"testing"

	"github.com/bearalliance/supergo/pkg/supergo"
)

func TestAgentCookiePersistence(t *testing.T) {
	agent := supergo.NewAgent(testMux())

	agent.Post("/login").
		Expect(200).
		Test(t)

	agent.Get("/profile").
		Expect(200).
		ExpectBodyContainsJSON("name", "alice").
		Test(t)
}

func TestAgentCookieDeletion(t *testing.T) {
	agent := supergo.NewAgent(testMux())

	agent.Post("/login").Expect(200).Test(t)
	agent.Delete("/logout").Expect(200).Test(t)
	agent.Get("/profile").Expect(401).Test(t)
}

func TestAgentHistory(t *testing.T) {
	agent := supergo.NewAgent(testMux())

	agent.Post("/login").Expect(200).Test(t)
	agent.Get("/profile").Expect(200).Test(t)

	history := agent.History()
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if history[0].Method != "POST" || history[0].Path != "/login" {
		t.Errorf("unexpected first history entry: %s %s", history[0].Method, history[0].Path)
	}
	if history[1].Method != "GET" || history[1].Path != "/profile" {
		t.Errorf("unexpected second history entry: %s %s", history[1].Method, history[1].Path)
	}
	if history[0].Response.StatusCode != 200 {
		t.Errorf("expected first response status 200, got %d", history[0].Response.StatusCode)
	}
	if len(history[0].Assertions) == 0 {
		t.Error("expected assertions to be recorded in history")
	}
}

func TestAgentHistoryAssertionNames(t *testing.T) {
	agent := supergo.NewAgent(testMux())

	agent.Get("/users").
		Expect(200).
		ExpectHeader("Content-Type", "application/json").
		Test(t)

	history := agent.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	assertions := history[0].Assertions
	if len(assertions) != 2 {
		t.Fatalf("expected 2 assertion names, got %d: %v", len(assertions), assertions)
	}
	if assertions[0] != "status 200" {
		t.Errorf("expected first assertion name 'status 200', got %q", assertions[0])
	}
}

func TestAgentClearHistory(t *testing.T) {
	agent := supergo.NewAgent(testMux())
	agent.Get("/users").Expect(200).Test(t)
	agent.ClearHistory()
	if len(agent.History()) != 0 {
		t.Error("expected empty history after ClearHistory")
	}
}
