// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"strconv"
	"testing"
	"time"
)

var testEscalation = Escalation{PerMinute: 5, BlockAfter: 5, Cooldown: 15 * time.Minute, MaxCooldown: 24 * time.Hour}

// newMinuteClock starts on a minute boundary, so a refusal's wait to the next
// minute is a whole minute.
func newMinuteClock() *fakeClock { return &fakeClock{t: time.Unix(28_333_334*60, 0)} }

func newTestEscalator(name string, clock *fakeClock) *Escalator {
	return NewRegistry().NewEscalator(name, FailClosed, testEscalation, clock.Now)
}

// overOneMinute sends one post past the limit in the current minute and
// answers that post's verdict.
func overOneMinute(t *testing.T, e *Escalator, key string) Verdict {
	t.Helper()
	for i := range testEscalation.PerMinute {
		if v := e.Check(key, ""); v.Refused {
			t.Fatalf("post %d of %d refused: %+v", i+1, testEscalation.PerMinute, v)
		}
	}
	return e.Check(key, "")
}

func TestARefusalPastTheLimitWaitsLongerWithEachFurtherPost(t *testing.T) {
	clock := newMinuteClock()
	e := newTestEscalator("test/escalate-wait", clock)
	first := overOneMinute(t, e, "ip")
	if !first.Refused || first.RetryAfter != time.Minute || first.Change != StartedRefusal || !first.BlockedUntil.IsZero() {
		t.Fatalf("first refusal = %+v, want a one-minute wait that starts the refusals", first)
	}
	want := []time.Duration{2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	for i, w := range want {
		if v := e.Check("ip", ""); !v.Refused || v.RetryAfter != w || v.Change != NoChange {
			t.Fatalf("refusal %d = %+v, want a wait of %s", i+2, v, w)
		}
	}
	if v := e.Check("other", ""); v.Refused {
		t.Fatalf("another key shares the refused key's minute: %+v", v)
	}
	clock.advance(time.Minute)
	if v := e.Check("ip", ""); v.Refused {
		t.Fatalf("a new minute still refuses: %+v", v)
	}
}

func TestFiveMinutesOverInARowBlockUntilACooldownThatDoublesOnRepeat(t *testing.T) {
	clock := newMinuteClock()
	e := newTestEscalator("test/escalate-block", clock)
	for minute := range testEscalation.BlockAfter - 1 {
		if v := overOneMinute(t, e, "form"); v.Change == StartedBlock {
			t.Fatalf("blocked after %d minutes over", minute+1)
		}
		clock.advance(time.Minute)
	}
	blocked := overOneMinute(t, e, "form")
	until := clock.Now().Add(15 * time.Minute)
	if !blocked.Refused || blocked.Change != StartedBlock || !blocked.BlockedUntil.Equal(until) || blocked.RetryAfter != 15*time.Minute {
		t.Fatalf("fifth minute over = %+v, want a 15-minute block", blocked)
	}
	clock.advance(14 * time.Minute)
	if v := e.Check("form", ""); !v.Refused || !v.BlockedUntil.Equal(until) || v.RetryAfter != time.Minute {
		t.Fatalf("a post inside the block = %+v, want refused until %s", v, until)
	}
	if _, blockedUntil, err := e.State("form"); err != nil || !blockedUntil.Equal(until) {
		t.Fatalf("State = %s, %v; want blocked until %s", blockedUntil, err, until)
	}
	clock.advance(time.Minute)
	if v := e.Check("form", ""); v.Refused {
		t.Fatalf("the block did not expire on its own: %+v", v)
	}

	clock.advance(time.Minute)
	for range testEscalation.BlockAfter - 1 {
		overOneMinute(t, e, "form")
		clock.advance(time.Minute)
	}
	again := overOneMinute(t, e, "form")
	if again.Change != StartedBlock || again.RetryAfter != 30*time.Minute {
		t.Fatalf("second block = %+v, want twice the first cooldown", again)
	}
}

func TestAQuietMinuteBetweenFloodsStartsTheCountAgain(t *testing.T) {
	clock := newMinuteClock()
	e := newTestEscalator("test/escalate-gap", clock)
	for range testEscalation.BlockAfter - 1 {
		overOneMinute(t, e, "ip")
		clock.advance(time.Minute)
	}
	clock.advance(time.Minute)
	if v := overOneMinute(t, e, "ip"); v.Change == StartedBlock {
		t.Fatalf("minutes over with a quiet one between them blocked: %+v", v)
	}
}

func TestTheStateNamesWhenRefusalsBeganAndForgetsThemOnceQuiet(t *testing.T) {
	clock := newMinuteClock()
	e := newTestEscalator("test/escalate-state", clock)
	began := clock.Now()
	overOneMinute(t, e, "form")
	clock.advance(30 * time.Second)
	e.Check("form", "")
	if since, until, err := e.State("form"); err != nil || !since.Equal(began) || !until.IsZero() {
		t.Fatalf("State = %s, %s, %v; want refused since %s", since, until, err, began)
	}
	clock.advance(3 * time.Minute)
	if since, _, err := e.State("form"); err != nil || !since.IsZero() {
		t.Fatalf("State after quiet minutes = %s, %v; want no refusal", since, err)
	}
}

func TestANotedRefusalShowsInTheStateWithoutCountingAPost(t *testing.T) {
	clock := newMinuteClock()
	e := newTestEscalator("test/escalate-note", clock)
	e.NoteRefusal("form")
	if since, _, err := e.State("form"); err != nil || !since.Equal(clock.Now()) {
		t.Fatalf("State = %s, %v; want the noted refusal", since, err)
	}
	for i := range testEscalation.PerMinute {
		if v := e.Check("form", ""); v.Refused {
			t.Fatalf("post %d refused after a note: %+v", i+1, v)
		}
	}
}

func TestAnEscalatorThatCannotReadItsStoreFollowsItsKind(t *testing.T) {
	clock := newMinuteClock()
	closed := NewRegistry().NewEscalator("test/escalate-closed", FailClosed, testEscalation, clock.Now)
	closed.meter.reg.shared = &unreachableStore{}
	if v := closed.Check("ip", ""); !v.Refused || v.RetryAfter != blindWait {
		t.Fatalf("fail-closed escalator without its store = %+v, want refused", v)
	}
	open := NewRegistry().NewEscalator("test/escalate-open", FailOpen, testEscalation, clock.Now)
	open.meter.reg.shared = &unreachableStore{}
	if v := open.Check("ip", ""); v.Refused {
		t.Fatalf("fail-open escalator without its store = %+v, want admitted", v)
	}
}

func TestABlockThatNeedsManySendersStaysA429WhileOneSenderFloods(t *testing.T) {
	clock := newMinuteClock()
	esc := testEscalation
	esc.MinSenders = 10
	esc.PerMinute = 2 * esc.MinSenders
	e := NewRegistry().NewEscalator("test/escalate-senders", FailClosed, esc, clock.Now)
	flood := func(senders int) Verdict {
		var last Verdict
		for i := range 4 * esc.MinSenders {
			last = e.Check("workspace", "sender-"+strconv.Itoa(i%senders))
		}
		return last
	}
	for range esc.BlockAfter {
		if v := flood(1); !v.Refused {
			t.Fatalf("one sender past the limit = %+v, want a 429", v)
		}
		if _, blocked := e.BlockedUntil("workspace"); blocked {
			t.Fatal("one sender blocked the key")
		}
		clock.advance(time.Minute)
	}
	many := NewRegistry().NewEscalator("test/escalate-many", FailClosed, esc, clock.Now)
	var last Verdict
	for range esc.BlockAfter {
		for i := range esc.PerMinute + 1 {
			last = many.Check("workspace", "sender-"+strconv.Itoa(i%10))
		}
		clock.advance(time.Minute)
	}
	if last.Change != StartedBlock {
		t.Fatalf("ten senders over five minutes = %+v, want a block", last)
	}
}

func TestReadingABlockCountsNoPost(t *testing.T) {
	clock := newMinuteClock()
	e := newTestEscalator("test/escalate-peek", clock)
	for range 3 * testEscalation.PerMinute {
		if _, refused := e.BlockedUntil("form"); refused {
			t.Fatal("an unblocked key reads as blocked")
		}
	}
	if v := e.Check("form", ""); v.Refused {
		t.Fatalf("reads spent the key's minute: %+v", v)
	}
}
