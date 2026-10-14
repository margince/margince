// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package events

// A handler whose subject is gone has nothing a retry can find. Left pending,
// its entry is reclaimed on every cycle for as long as the stream keeps it.
//
// Eleven such entries on one staging installation retried every five minutes
// across hours. That is the noise which hides a lane about to recover.

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUnactionableEntryIsAckedRatherThanRetried(t *testing.T) {
	e := setup(t)
	e.stage(t, "contact.created", ids.NewV7())
	if _, err := e.relay(t).relayBatch(t.Context()); err != nil {
		t.Fatal(err)
	}

	var seen atomic.Int32
	group := kevents.Group{Name: "cg:gone-subject", Streams: []string{"gw:events:crm:contact"}}
	s := NewSubscriber(e.rdb, group, func(_ context.Context, env kevents.Envelope) error {
		seen.Add(1)
		return fmt.Errorf("%w: contact %s", kevents.ErrUnactionable, env.Entity.ID)
	}, testLogger())
	s.block = 100 * time.Millisecond

	pendingCount := func(t *testing.T) int64 {
		t.Helper()
		pending, err := e.rdb.XPending(t.Context(), "gw:events:crm:contact", group.Name).Result()
		if err != nil {
			t.Fatalf("reading the pending list: %v", err)
		}
		return pending.Count
	}
	consumeUntil(t, s, 5*time.Second, func() bool { return seen.Load() >= 1 && pendingCount(t) == 0 })

	if got := seen.Load(); got < 1 {
		t.Fatal("the handler never ran, so this proves nothing about what its answer does")
	}
	if n := pendingCount(t); n != 0 {
		t.Errorf("%d entries still pending; an entry whose subject is gone is acked, not reclaimed forever", n)
	}
}

// The other direction, and the one the consumer exists for. An ordinary failure
// is still retried.
//
// A handler reporting every error as unactionable would ack away the ordering
// race this bus absorbs.
func TestAnOrdinaryFailureStaysPending(t *testing.T) {
	e := setup(t)
	e.stage(t, "contact.created", ids.NewV7())
	if _, err := e.relay(t).relayBatch(t.Context()); err != nil {
		t.Fatal(err)
	}

	var seen atomic.Int32
	group := kevents.Group{Name: "cg:transient", Streams: []string{"gw:events:crm:contact"}}
	s := NewSubscriber(e.rdb, group, func(_ context.Context, _ kevents.Envelope) error {
		seen.Add(1)
		return errors.New("the contact store is unreachable")
	}, testLogger())
	s.block = 100 * time.Millisecond

	consumeUntil(t, s, 3*time.Second, func() bool { return seen.Load() >= 1 })
	pending, err := e.rdb.XPending(t.Context(), "gw:events:crm:contact", group.Name).Result()
	if err != nil {
		t.Fatalf("reading the pending list: %v", err)
	}
	if pending.Count == 0 {
		t.Error("an ordinary failure left nothing pending, so a transient fault would be acked away")
	}
}
