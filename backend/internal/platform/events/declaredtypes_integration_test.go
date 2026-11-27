// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package events

// An entry of a type its group does not declare is acked. The handler never
// runs on it, and no dedupe mark is written for it.

import (
	"testing"
	"time"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUndeclaredTypeIsAckedWithoutADedupeMark(t *testing.T) {
	t.Parallel()
	e := setup(t)
	const group = "cg:declared-types"
	stream := "gw:events:crm:deal"
	for range 3 {
		e.stageDealEvent(t, "deal.updated", `{}`)
	}
	handled := e.stageDealEvent(t, "deal.stage_changed", `{"to_stage_id":"`+ids.NewV7().String()+`"}`)
	if _, err := e.relay(t).relayBatch(t.Context()); err != nil {
		t.Fatal(err)
	}

	seen := &collector{seen: map[ids.UUID]bool{}}
	sub := NewSubscriber(e.rdb, kevents.Group{
		Name: group, Streams: []string{stream}, Types: kevents.NewTypeSet("deal.stage_changed"),
	}, Dedupe(e.rdb, group, seen.handle), testLogger())
	sub.block = 100 * time.Millisecond

	pending := func() int64 {
		p, err := e.rdb.XPending(t.Context(), stream, group).Result()
		if err != nil {
			t.Fatalf("reading the pending list: %v", err)
		}
		return p.Count
	}
	consumeUntil(t, sub, 5*time.Second, func() bool { return seen.has(handled) && pending() == 0 })

	if len(seen.seen) != 1 || !seen.has(handled) {
		t.Errorf("the handler ran on %d event(s), want only the declared deal.stage_changed", len(seen.seen))
	}
	if n := pending(); n != 0 {
		t.Errorf("%d entries pending: an undeclared type is acked, or the reclaim pass redelivers it forever", n)
	}
	keys, err := e.rdb.Keys(t.Context(), DedupeKeyPrefix+group+":*").Result()
	if err != nil {
		t.Fatalf("listing dedupe marks: %v", err)
	}
	if len(keys) != 1 || keys[0] != DedupeKeyPrefix+group+":"+handled.String() {
		t.Errorf("dedupe marks %v, want exactly the handled event's", keys)
	}
}
