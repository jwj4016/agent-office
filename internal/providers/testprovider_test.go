package providers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func loadFixtures(t *testing.T) *TestProvider {
	t.Helper()
	p, err := LoadTestProvider("../../tests/fixtures/providers")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func req(scenario string) StartRequest {
	return StartRequest{ProjectID: "p1", RunID: "r1", StepAttemptID: "s1", Generation: 2, Model: scenario}
}

// next reads one event or fails after a timeout.
func next(t *testing.T, s Session) Event {
	t.Helper()
	select {
	case ev, ok := <-s.Events():
		if !ok {
			t.Fatal("events closed early")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event")
	}
	return Event{}
}

// drain collects events until the channel closes.
func drain(t *testing.T, s Session) []Event {
	t.Helper()
	var out []Event
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			t.Fatal("timed out draining events")
		}
	}
}

func completed(t *testing.T, evs []Event) CompletedPayload {
	t.Helper()
	last := evs[len(evs)-1]
	if last.Kind != KindCompleted {
		t.Fatalf("last event is %s, want completed", last.Kind)
	}
	var p CompletedPayload
	json.Unmarshal(last.Payload, &p)
	return p
}

func TestSuccessStreamIsStampedAndOrdered(t *testing.T) {
	s, err := loadFixtures(t).Start(context.Background(), req("plan-success"))
	if err != nil {
		t.Fatal(err)
	}
	evs := drain(t, s)
	kinds := []string{}
	for i, ev := range evs {
		kinds = append(kinds, ev.Kind)
		if ev.Sequence != int64(i+1) || ev.ProjectID != "p1" || ev.RunID != "r1" ||
			ev.StepAttemptID != "s1" || ev.Generation != 2 || ev.EventID == "" || ev.SchemaVersion != EventSchemaVersion {
			t.Fatalf("event %d badly stamped: %+v", i, ev)
		}
	}
	want := []string{KindStarted, KindMessageDelta, KindMessageDelta, KindMessage, KindUsage, KindCompleted}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v", kinds)
	}
	var u UsagePayload
	json.Unmarshal(evs[4].Payload, &u)
	if u.CostUSD != nil {
		t.Fatal("unknown cost must stay nil, not 0")
	}
	if p := completed(t, evs); p.Status != StatusSucceeded {
		t.Fatalf("status = %s", p.Status)
	}
}

func TestApprovalWaitsForResponse(t *testing.T) {
	ctx := context.Background()
	s, _ := loadFixtures(t).Start(ctx, req("tool-approval"))
	next(t, s) // started
	next(t, s) // message
	ev := next(t, s)
	if ev.Kind != KindApprovalRequest {
		t.Fatalf("got %s", ev.Kind)
	}
	select {
	case ev := <-s.Events():
		t.Fatalf("progressed before approval: %s", ev.Kind)
	case <-time.After(100 * time.Millisecond):
	}
	if err := s.Respond(ctx, Response{RequestID: "wrong"}); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("want ErrUnknownRequest, got %v", err)
	}
	if err := s.Respond(ctx, Response{RequestID: "req-1", Decision: DecisionAccept}); err != nil {
		t.Fatal(err)
	}
	if err := s.Respond(ctx, Response{RequestID: "req-1", Decision: DecisionAccept}); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("second response must be rejected, got %v", err)
	}
	rest := drain(t, s)
	if rest[0].Kind != KindRequestResolved || completed(t, rest).Status != StatusSucceeded {
		t.Fatalf("unexpected tail: %+v", rest)
	}
}

func TestCancelStopsLongStep(t *testing.T) {
	ctx := context.Background()
	s, _ := loadFixtures(t).Start(ctx, req("slow"))
	next(t, s)
	next(t, s)
	start := time.Now()
	s.Cancel(ctx)
	s.Cancel(ctx) // idempotent
	evs := drain(t, s)
	if completed(t, evs).Status != StatusCancelled || time.Since(start) > time.Second {
		t.Fatalf("cancel not honored promptly: %+v", evs)
	}
	if err := s.Respond(ctx, Response{RequestID: "x"}); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("respond after close: %v", err)
	}
}

func TestClaimedDoneWithoutCompletionIsFailure(t *testing.T) {
	s, _ := loadFixtures(t).Start(context.Background(), req("claims-done-no-complete"))
	if p := completed(t, drain(t, s)); p.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", p.Status)
	}
}

func TestStartValidation(t *testing.T) {
	p := loadFixtures(t)
	if _, err := p.Start(context.Background(), StartRequest{Model: "plan-success"}); err == nil {
		t.Fatal("missing ids accepted")
	}
	if _, err := p.Start(context.Background(), req("nope")); err == nil {
		t.Fatal("unknown scenario accepted")
	}
	if _, err := p.Resume(context.Background(), req("plan-success"), "x"); !errors.Is(err, ErrUnsupported) {
		t.Fatal("resume must be unsupported")
	}
}

func TestParseScriptRejectsBadLines(t *testing.T) {
	for _, bad := range []string{
		`{"request": "question", "payload": {}}`,
		`{"sleep": "soon"}`,
		`not json`,
	} {
		if _, err := ParseScript([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
