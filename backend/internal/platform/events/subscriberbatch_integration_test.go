// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package events

// Coalesced delivery against a real Redis. The properties, in the order they
// matter: a backlog reaches the batch handler as a few full reads rather than
// one call per entry; every entry is still acked exactly when its effect ran;
// a failed batch falls through to the per-entry handler rather than stranding
// its entries; and the batch dedupe honours the per-event marks both ways.

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const batchStream = "gw:events:crm:contact"

// stageBurst stages and relays n contact events, as a capture burst leaves
// them waiting for a consumer that has fallen behind.
func (e *busEnv) stageBurst(t *testing.T, n int) []kevents.Envelope {
	t.Helper()
	envs := make([]kevents.Envelope, 0, n)
	for i := 0; i < n; i++ {
		envs = append(envs, e.stage(t, "contact.updated", ids.NewV7()))
	}
	for {
		published, err := e.relay(t).relayBatch(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if published == 0 {
			return envs
		}
	}
}

func (e *busEnv) pending(t *testing.T, group string) int64 {
	t.Helper()
	p, err := e.rdb.XPending(t.Context(), batchStream, group).Result()
	if err != nil {
		t.Fatalf("reading the pending list: %v", err)
	}
	return p.Count
}

func TestABacklogIsDeliveredAsFewBatchesNotOneCallPerEntry(t *testing.T) {
	e := setup(t)
	const burst = 150
	e.stageBurst(t, burst)

	group := kevents.Group{Name: "cg:graph-edge", Streams: []string{batchStream}}
	var singles, batches, batched atomic.Int32
	s := NewSubscriber(e.rdb, group, func(context.Context, kevents.Envelope) error {
		singles.Add(1)
		return nil
	}, testLogger()).WithBatch(func(_ context.Context, envs []kevents.Envelope) error {
		batches.Add(1)
		batched.Add(int32(len(envs)))
		return nil
	})
	s.block = 100 * time.Millisecond

	consumeUntil(t, s, 10*time.Second, func() bool {
		return singles.Load()+batched.Load() >= burst && e.pending(t, group.Name) == 0
	})

	if got := singles.Load() + batched.Load(); got != burst {
		t.Fatalf("%d entries reached a handler, want %d — each exactly once", got, burst)
	}
	// 150 waiting entries read 64 at a time: three reads, the last one short.
	// The effect runs once per READ, which is what spares a projection the
	// per-event refold of the pairs a burst shares.
	t.Logf("%d entries: %d batch calls carrying %d, %d single deliveries",
		burst, batches.Load(), batched.Load(), singles.Load())
	if got, want := batches.Load(), int32((burst+63)/64); got > want {
		t.Errorf("the backlog took %d batch calls, want at most %d", got, want)
	}
	if n := e.pending(t, group.Name); n != 0 {
		t.Errorf("%d entries still pending after their batch succeeded", n)
	}
}

func TestAFailedBatchFallsBackToOneEntryAtATime(t *testing.T) {
	e := setup(t)
	const burst = 5
	e.stageBurst(t, burst)

	group := kevents.Group{Name: "cg:graph-edge", Streams: []string{batchStream}}
	var singles, batches atomic.Int32
	s := NewSubscriber(e.rdb, group, func(context.Context, kevents.Envelope) error {
		singles.Add(1)
		return nil
	}, testLogger()).WithBatch(func(context.Context, []kevents.Envelope) error {
		batches.Add(1)
		return fmt.Errorf("simulated batch failure")
	})
	s.block = 100 * time.Millisecond

	consumeUntil(t, s, 10*time.Second, func() bool {
		return singles.Load() >= burst && e.pending(t, group.Name) == 0
	})

	if batches.Load() == 0 {
		t.Fatal("the batch handler never ran, so the fallback was not what delivered these")
	}
	// Without the fallback the entries would sit pending until the reclaim
	// window (minutes) and then fail as a batch again.
	if got := singles.Load(); got != burst {
		t.Errorf("the per-entry handler ran %d times, want %d — once per entry of the failed batch", got, burst)
	}
}

func TestTheBatchDedupeAndThePerEventDedupeShareTheirMarks(t *testing.T) {
	e := setup(t)
	envs := e.stageBurst(t, 3)
	const group = "cg:graph-edge"

	// The first event was processed one at a time, before the batch.
	if err := Dedupe(e.rdb, group, func(context.Context, kevents.Envelope) error { return nil })(t.Context(), envs[0]); err != nil {
		t.Fatal(err)
	}

	var got []ids.UUID
	fail := true
	batch := DedupeBatch(e.rdb, group, func(_ context.Context, fresh []kevents.Envelope) error {
		if fail {
			return fmt.Errorf("transient effect failure")
		}
		for _, env := range fresh {
			got = append(got, env.EventID)
		}
		return nil
	})
	if err := batch(t.Context(), envs); err == nil {
		t.Fatal("the batch failure was swallowed")
	}
	for _, env := range envs[1:] {
		if seen, _ := e.rdb.Exists(t.Context(), dedupeKey(group, env)).Result(); seen != 0 {
			t.Fatal("a mark exists although the batch never succeeded — a crash here would drop the event")
		}
	}

	fail = false
	if err := batch(t.Context(), envs); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(got) != 2 || got[0] != envs[1].EventID || got[1] != envs[2].EventID {
		t.Fatalf("the batch ran for %v, want only the two events the per-event path had not processed", got)
	}

	// And the other way round: what the batch marked, the per-event path skips.
	var ran atomic.Int32
	if err := Dedupe(e.rdb, group, func(context.Context, kevents.Envelope) error {
		ran.Add(1)
		return nil
	})(t.Context(), envs[2]); err != nil {
		t.Fatal(err)
	}
	if ran.Load() != 0 {
		t.Error("the per-event path re-ran an event the batch had already processed")
	}
}

// An entry that cannot be decoded can never succeed, so a batch acks it away
// rather than leaving it to poison the reclaim pass, and the entries around it
// still reach the handler. A read made only of such entries runs no effect.
func TestABatchDropsUndecodableEntriesAndDeliversTheRest(t *testing.T) {
	e := setup(t)
	group := kevents.Group{Name: "cg:graph-edge", Streams: []string{batchStream}}
	junk := func() {
		t.Helper()
		if err := e.rdb.XAdd(t.Context(), &redis.XAddArgs{
			Stream: batchStream, Values: map[string]any{envelopeField: "{not an envelope"},
		}).Err(); err != nil {
			t.Fatalf("XADD: %v", err)
		}
	}

	// A read of nothing but junk.
	junk()
	junk()
	var batches atomic.Int32
	var delivered atomic.Int32
	s := NewSubscriber(e.rdb, group, func(context.Context, kevents.Envelope) error {
		delivered.Add(1)
		return nil
	}, testLogger()).WithBatch(func(_ context.Context, envs []kevents.Envelope) error {
		batches.Add(1)
		delivered.Add(int32(len(envs)))
		return nil
	})
	s.block = 100 * time.Millisecond
	consumeUntil(t, s, 5*time.Second, func() bool { return e.pending(t, group.Name) == 0 })
	if batches.Load() != 0 || delivered.Load() != 0 {
		t.Fatalf("a read of undecodable entries reached a handler (%d batches, %d envelopes)", batches.Load(), delivered.Load())
	}

	// Junk between real events.
	junk()
	e.stageBurst(t, 3)
	junk()
	consumeUntil(t, s, 5*time.Second, func() bool { return delivered.Load() >= 3 && e.pending(t, group.Name) == 0 })
	if got := delivered.Load(); got != 3 {
		t.Errorf("%d envelopes reached a handler, want the 3 decodable ones", got)
	}
}

// A dedupe check that cannot reach Redis must not run the effect, and a mark
// that cannot be written must fail the batch, so the per-entry path re-runs
// the effect and retries its own mark.
func TestTheBatchDedupeFailsClosedWhenRedisDoes(t *testing.T) {
	e := setup(t)
	envs := e.stageBurst(t, 2)
	const group = "cg:graph-edge"

	var ran atomic.Int32
	all := DedupeBatch(e.rdb, group, func(context.Context, []kevents.Envelope) error {
		ran.Add(1)
		return nil
	})
	if err := all(t.Context(), envs); err != nil {
		t.Fatal(err)
	}
	if err := all(t.Context(), envs); err != nil {
		t.Fatal(err)
	}
	if ran.Load() != 1 {
		t.Fatalf("the effect ran %d times, want 1 — a batch already processed in full runs nothing", ran.Load())
	}

	down := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = down.Close() }) //craft:ignore swallowed-errors a client that never connected has nothing to flush
	if err := DedupeBatch(down, group, func(context.Context, []kevents.Envelope) error {
		t.Error("the effect ran although the dedupe check failed")
		return nil
	})(t.Context(), envs); err == nil {
		t.Error("an unreachable dedupe store was treated as nothing processed yet")
	}

	lost := redis.NewClient(e.rdb.Options())
	fresh := e.stageBurst(t, 1)
	err := DedupeBatch(lost, group, func(context.Context, []kevents.Envelope) error {
		return lost.Close() // the store goes away between the effect and its mark
	})(t.Context(), fresh)
	if err == nil {
		t.Fatal("a mark that could not be written was reported as success")
	}
	if seen, _ := e.rdb.Exists(t.Context(), dedupeKey(group, fresh[0])).Result(); seen != 0 {
		t.Error("a mark exists for a batch whose marking failed")
	}
}
