package fsm

import (
	"reflect"
	"strings"
	"testing"
)

// orderDef returns a valid workflow definition used throughout tests (order fulfillment).
// pending --pay--> paid --ship--> shipped --deliver--> delivered
// pending --cancel--> cancelled ; paid --cancel--> cancelled
// Terminal states: delivered, cancelled.
func orderDef() Definition {
	return Definition{
		Name:    "order-fulfillment",
		States:  []string{"pending", "paid", "shipped", "delivered", "cancelled"},
		Initial: "pending",
		Transitions: []Transition{
			{From: "pending", Event: "pay", To: "paid"},
			{From: "paid", Event: "ship", To: "shipped"},
			{From: "shipped", Event: "deliver", To: "delivered"},
			{From: "pending", Event: "cancel", To: "cancelled"},
			{From: "paid", Event: "cancel", To: "cancelled"},
		},
	}
}

func TestValidateOK(t *testing.T) {
	if err := orderDef().Validate(); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}
}

func TestValidateEmptyName(t *testing.T) {
	d := orderDef()
	d.Name = "  "
	if err := d.Validate(); err == nil {
		t.Error("empty name should be rejected")
	}
}

func TestValidateEmptyStates(t *testing.T) {
	d := orderDef()
	d.States = nil
	if err := d.Validate(); err == nil {
		t.Error("empty states should be rejected")
	}
}

func TestValidateDuplicateStates(t *testing.T) {
	d := orderDef()
	d.States = []string{"pending", "paid", "pending"}
	d.Initial = "pending"
	d.Transitions = nil
	if err := d.Validate(); err == nil {
		t.Error("duplicate state names should be rejected")
	}
}

func TestValidateInitialNotInStates(t *testing.T) {
	d := orderDef()
	d.Initial = "nowhere"
	if err := d.Validate(); err == nil {
		t.Error("initial not in states should be rejected")
	}
}

func TestValidateTransitionFromNotInStates(t *testing.T) {
	d := orderDef()
	d.Transitions = append(d.Transitions, Transition{From: "ghost", Event: "x", To: "paid"})
	if err := d.Validate(); err == nil {
		t.Error("transition from undeclared state should be rejected")
	}
}

func TestValidateTransitionToNotInStates(t *testing.T) {
	d := orderDef()
	d.Transitions = append(d.Transitions, Transition{From: "paid", Event: "x", To: "ghost"})
	if err := d.Validate(); err == nil {
		t.Error("transition to undeclared state should be rejected")
	}
}

func TestValidateDuplicateTransitionDifferentTarget(t *testing.T) {
	// Same (from, event) pointing to different targets: non-deterministic, must reject.
	d := orderDef()
	d.Transitions = append(d.Transitions, Transition{From: "pending", Event: "pay", To: "shipped"})
	err := d.Validate()
	if err == nil {
		t.Fatal("duplicate (from,event) with different target should be rejected")
	}
	if !strings.Contains(err.Error(), "pending") || !strings.Contains(err.Error(), "pay") {
		t.Errorf("error should name state and event, got: %v", err)
	}
}

func TestValidateDuplicateTransitionSameTarget(t *testing.T) {
	// Even if the target is the same, duplicate (from, event) must be rejected.
	d := orderDef()
	d.Transitions = append(d.Transitions, Transition{From: "pending", Event: "pay", To: "paid"})
	if err := d.Validate(); err == nil {
		t.Error("duplicate (from,event) even with same target should be rejected")
	}
}

func TestValidateMissingTransitionField(t *testing.T) {
	d := orderDef()
	d.Transitions = append(d.Transitions, Transition{From: "paid", Event: "", To: "shipped"})
	if err := d.Validate(); err == nil {
		t.Error("transition with missing field should be rejected")
	}
}

func TestApplyOK(t *testing.T) {
	d := orderDef()
	next, ok, reason := d.Apply("pending", "pay")
	if !ok || next != "paid" || reason != "" {
		t.Errorf("Apply(pending,pay) = (%q,%v,%q), want (paid,true,\"\")", next, ok, reason)
	}
}

func TestApplyUndefinedEvent(t *testing.T) {
	// paid has ship/cancel but not deliver: undefined transition.
	d := orderDef()
	next, ok, reason := d.Apply("paid", "deliver")
	if ok || next != "paid" {
		t.Errorf("undefined event should not advance: next=%q ok=%v", next, ok)
	}
	if !strings.Contains(reason, "paid") || !strings.Contains(reason, "deliver") {
		t.Errorf("reason should name state and event, got: %q", reason)
	}
	if !strings.Contains(reason, "未定义转移") {
		t.Errorf("reason should say undefined transition, got: %q", reason)
	}
}

func TestApplyTerminalRejectsAll(t *testing.T) {
	// delivered is terminal: rejects all events.
	d := orderDef()
	for _, ev := range []string{"pay", "ship", "deliver", "cancel", "anything"} {
		next, ok, reason := d.Apply("delivered", ev)
		if ok || next != "delivered" {
			t.Errorf("terminal should reject event %q: next=%q ok=%v", ev, next, ok)
		}
		if !strings.Contains(reason, "终态") || !strings.Contains(reason, "delivered") {
			t.Errorf("terminal reason should mention terminal+state for event %q, got: %q", ev, reason)
		}
	}
}

func TestApplyTerminalAndUndefinedErrorsDiffer(t *testing.T) {
	d := orderDef()
	_, _, terminalReason := d.Apply("delivered", "pay")
	_, _, undefinedReason := d.Apply("paid", "deliver")
	if terminalReason == undefinedReason {
		t.Errorf("terminal and undefined reasons must differ: both=%q", terminalReason)
	}
	if !strings.Contains(terminalReason, "终态") {
		t.Errorf("terminal reason missing 终态: %q", terminalReason)
	}
	if !strings.Contains(undefinedReason, "未定义转移") {
		t.Errorf("undefined reason missing 未定义转移: %q", undefinedReason)
	}
}

func TestApplyUnknownState(t *testing.T) {
	d := orderDef()
	next, ok, reason := d.Apply("ghost", "pay")
	if ok || next != "ghost" {
		t.Errorf("unknown state should not advance: next=%q ok=%v", next, ok)
	}
	if !strings.Contains(reason, "ghost") {
		t.Errorf("reason should name unknown state, got: %q", reason)
	}
}

func TestTerminals(t *testing.T) {
	d := orderDef()
	got := d.Terminals()
	want := []string{"cancelled", "delivered"} // sorted ascending
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Terminals = %v, want %v", got, want)
	}
}

func TestTerminalsInDegreeIrrelevant(t *testing.T) {
	// cancelled has two incoming edges but no outgoing edges; still terminal.
	d := orderDef()
	terms := d.Terminals()
	found := false
	for _, s := range terms {
		if s == "cancelled" {
			found = true
		}
	}
	if !found {
		t.Errorf("cancelled has in-edges but no out-edges, should be terminal: %v", terms)
	}
}

func TestTerminalsNoneCycle(t *testing.T) {
	// Pure cycle: every state has outgoing edges, no terminals.
	d := Definition{
		Name:    "cycle",
		States:  []string{"a", "b"},
		Initial: "a",
		Transitions: []Transition{
			{From: "a", Event: "x", To: "b"},
			{From: "b", Event: "y", To: "a"},
		},
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("cycle workflow should be valid: %v", err)
	}
	if got := d.Terminals(); len(got) != 0 {
		t.Errorf("cycle should have no terminals, got %v", got)
	}
}

func TestPathDirect(t *testing.T) {
	d := orderDef()
	reach, events := d.Path("pending", "cancelled")
	if !reach {
		t.Fatal("pending->cancelled should be reachable")
	}
	if !reflect.DeepEqual(events, []string{"cancel"}) {
		t.Errorf("direct path events = %v, want [cancel]", events)
	}
}

func TestPathMultiHopShortest(t *testing.T) {
	d := orderDef()
	reach, events := d.Path("pending", "delivered")
	if !reach {
		t.Fatal("pending->delivered should be reachable")
	}
	want := []string{"pay", "ship", "deliver"}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("multi-hop path = %v, want %v", events, want)
	}
}

func TestPathSelf(t *testing.T) {
	d := orderDef()
	reach, events := d.Path("pending", "pending")
	if !reach {
		t.Fatal("self should be reachable")
	}
	if len(events) != 0 {
		t.Errorf("self path events should be empty, got %v", events)
	}
}

func TestPathForwardOnly(t *testing.T) {
	// pending->delivered is reachable forward, but delivered->pending is not (terminal).
	d := orderDef()
	reach, events := d.Path("delivered", "pending")
	if reach {
		t.Errorf("delivered->pending should be unreachable forward, got events=%v", events)
	}
}

func TestPathUnreachable(t *testing.T) {
	// delivered and cancelled are mutually unreachable (both terminal).
	d := orderDef()
	reach, _ := d.Path("delivered", "cancelled")
	if reach {
		t.Error("delivered->cancelled should be unreachable")
	}
}

func TestPathShortestAmongAlternatives(t *testing.T) {
	// pending can reach cancelled via direct cancel OR pay->cancel.
	// BFS should return the shorter path [cancel].
	d := orderDef()
	reach, events := d.Path("pending", "cancelled")
	if !reach || len(events) != 1 || events[0] != "cancel" {
		t.Errorf("shortest path = %v (reach=%v), want [cancel]", events, reach)
	}
}
