// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"errors"
	"testing"
	"time"
)

// errUnreachable stands for what a shared store returns when it cannot be
// reached — a dialled-but-dead Redis, a timeout, a failover. What it says does
// not matter to the limiter; that it could not count is the whole input.
var errUnreachable = errors.New("shared store unreachable")

// unreachableStore fails every operation, so a limiter built on it is asked
// the one question this file exists for: with no count available, what do you
// answer?
type unreachableStore struct{ counted int }

func (s *unreachableStore) count(string, time.Duration, time.Time) (int, error) {
	s.counted++
	return 0, errUnreachable
}

func (s *unreachableStore) peek(string, time.Duration, time.Time) (int, error) {
	return 0, errUnreachable
}
func (s *unreachableStore) forget(string) error { return errUnreachable }

// unreachable builds a registry whose shared store is bound and broken, which
// is the state a Redis outage puts every limiter in this process into.
func unreachable() (*Registry, *unreachableStore) {
	s := &unreachableStore{}
	return &Registry{shared: s}, s
}

// A FailClosed ceiling is the last thing between an attacker and a credential,
// a link or a tenant. If it admitted when it could not count, the first move
// against every one of them would be the same move — make Redis unavailable —
// and the ceilings would all open together.
func TestACeilingBuiltToFailClosedRefusesWhenItCannotCount(t *testing.T) {
	reg, _ := unreachable()
	l := reg.New("test/security", FailClosed, 100, time.Minute)

	if l.Allow("alice") {
		t.Error("Allow admitted on a limiter that could not count; a ceiling an attacker can switch off is not a ceiling")
	}
	if !l.Blocked("alice") {
		t.Error("Blocked reported the key clear on a limiter that could not read it")
	}
}

// A FailOpen bound protects throughput for a caller already past
// authentication. Refusing there would convert a dependency blip into an
// outage of the surface being paced, and nothing it protects becomes unsafe
// for a window of unmetered traffic.
func TestABoundBuiltToFailOpenAdmitsWhenItCannotCount(t *testing.T) {
	reg, _ := unreachable()
	l := reg.New("test/pacing", FailOpen, 100, time.Minute)

	if !l.Allow("alice") {
		t.Error("Allow refused on a pacing bound; an unreachable counter must not take the surface down")
	}
	if l.Blocked("alice") {
		t.Error("Blocked reported a pacing bound spent when it could not read it")
	}
}

// The key bound is checked before the store is, on every entry point. An
// over-long key is one a client chose the size of, so a FailClosed limiter that
// refused it would hand any caller a way to lock out whatever the key names.
func TestAnUnmeterableKeyIsStillNotRefusedWhenTheStoreIsDown(t *testing.T) {
	reg, store := unreachable()
	l := reg.New("test/oversized", FailClosed, 1, time.Minute)
	oversized := string(make([]byte, maxKeyLen+1))

	if !l.Allow(oversized) {
		t.Error("Allow refused an unmeterable key; refusing to meter must not refuse the caller")
	}
	if l.Blocked(oversized) {
		t.Error("Blocked reported an unmeterable key as spent")
	}
	l.Record(oversized)
	if store.counted != 0 {
		t.Errorf("the store was asked to count %d unmeterable keys, want 0", store.counted)
	}
}

// Binding a shared store must MOVE the counting, not add a tier in front of
// it: a limiter that kept answering from its own map would be back to one
// ceiling per replica, and would look correct in every single-process test.
func TestBindingASharedStoreTakesTheLocalOneOutOfThePath(t *testing.T) {
	reg := NewRegistry()
	l := reg.New("test/rebind", FailOpen, 1, time.Minute)

	if !l.Allow("k") {
		t.Fatal("the first attempt must be admitted while counting locally")
	}
	if l.Allow("k") {
		t.Fatal("the second attempt must be refused; the local bucket is spent")
	}

	broken, _ := unreachable()
	reg.RebindFrom(broken)

	if got := heldKeys(t, l); got == 0 {
		t.Fatal("the local store was emptied by the rebind, so this asserts nothing")
	}
	if !l.Allow("k") {
		t.Error("the limiter answered from its local bucket after a shared store was bound")
	}
}
