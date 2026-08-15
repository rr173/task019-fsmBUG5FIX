package fsm

import (
	"sort"
	"testing"
)

// TestProbeWhitespaceStateRejected verifies that state names consisting
// solely of whitespace are rejected by Validate as invalid.
func TestProbeWhitespaceStateRejected(t *testing.T) {
	d := Definition{
		Name:    "test",
		States:  []string{"ok", "  ", "done"},
		Initial: "ok",
		Transitions: []Transition{
			{From: "ok", Event: "finish", To: "done"},
		},
	}
	if err := d.Validate(); err == nil {
		t.Error("Validate should reject whitespace-only state name")
	}
}

// TestProbePathSelfReachable verifies that Path(x, x) returns true
// (a state is trivially reachable from itself with zero events).
func TestProbePathSelfReachable(t *testing.T) {
	d := Definition{
		Name:    "test",
		States:  []string{"a", "b"},
		Initial: "a",
		Transitions: []Transition{
			{From: "a", Event: "go", To: "b"},
		},
	}
	ok, events := d.Path("a", "a")
	if !ok {
		t.Error("Path(a, a) should return true (self is reachable)")
	}
	if len(events) != 0 {
		t.Errorf("Path(a, a) events = %v, want empty", events)
	}
}

// TestProbeTerminalsSorted verifies that Terminals() returns terminal
// states in sorted (ascending) order regardless of declaration order.
func TestProbeTerminalsSorted(t *testing.T) {
	d := Definition{
		Name:    "test",
		States:  []string{"z-end", "a-start", "m-end"},
		Initial: "a-start",
		Transitions: []Transition{
			{From: "a-start", Event: "go-z", To: "z-end"},
			{From: "a-start", Event: "go-m", To: "m-end"},
		},
	}
	terms := d.Terminals()
	if !sort.StringsAreSorted(terms) {
		t.Errorf("Terminals() = %v, want sorted ascending", terms)
	}
}

// TestProbeApplyUnknownStatePreservesInput verifies that Apply returns
// the original state value (not empty) when the state is unknown.
func TestProbeApplyUnknownStatePreservesInput(t *testing.T) {
	d := Definition{
		Name:    "test",
		States:  []string{"a"},
		Initial: "a",
	}
	next, ok, _ := d.Apply("unknown-state", "any")
	if ok {
		t.Fatal("Apply with unknown state should return ok=false")
	}
	if next != "unknown-state" {
		t.Errorf("Apply unknown state: next = %q, want %q", next, "unknown-state")
	}
}
