// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package events

// A group that does not react to history never sees an event an import wrote.
// A group that keeps imported events still gets every one.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stageDealEvent stages one deal.stage_changed with the payload given.
func (e *busEnv) stageDealEvent(t *testing.T, payload string) ids.UUID {
	t.Helper()
	env := kevents.Envelope{
		EventID: ids.NewV7(), Type: "deal.stage_changed", Version: kevents.VersionOf("deal.stage_changed"),
		OccurredAt: clocktest.Now(t),
		Actor:      kevents.Actor{Type: "human", ID: "human:" + ids.NewV7().String()},
		Entity:     kevents.EntityRef{Type: "deal", ID: ids.NewV7()},
		Payload:    json.RawMessage(payload),
		Trace:      kevents.Trace{CorrelationID: ids.NewV7(), AuditLogID: ids.NewV7()},
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("fixture envelope: %v", err)
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(t.Context(),
		`INSERT INTO event_outbox (stream, envelope) VALUES ($1, $2)`, "gw:events:crm:deal", raw); err != nil {
		t.Fatalf("staging outbox row: %v", err)
	}
	return env.EventID
}

// collector remembers which events a handler was given.
type collector struct {
	mu   sync.Mutex
	seen map[ids.UUID]bool
}

func (c *collector) handle(_ context.Context, env kevents.Envelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen[env.EventID] = true
	return nil
}

func (c *collector) has(id ids.UUID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seen[id]
}

func TestAnImportedEventReachesOnlyTheGroupsThatKeepImportedEvents(t *testing.T) {
	e := setup(t)
	imported := e.stageDealEvent(t, `{"to_stage_id":"`+ids.NewV7().String()+`","import_run_id":"`+ids.NewV7().String()+`","imported":true}`)
	ordinary := e.stageDealEvent(t, `{"to_stage_id":"`+ids.NewV7().String()+`"}`)
	if _, err := e.relay(t).relayBatch(t.Context()); err != nil {
		t.Fatal(err)
	}

	skipping, keeping := &collector{seen: map[ids.UUID]bool{}}, &collector{seen: map[ids.UUID]bool{}}
	// The commissions group skips imported events, and the read model keeps them.
	skips := NewSubscriber(e.rdb, kevents.Group{Name: "cg:commissions", Streams: []string{"gw:events:crm:deal"}}, skipping.handle, testLogger())
	keeps := NewSubscriber(e.rdb, kevents.Group{Name: "cg:read-model", Streams: []string{"gw:events:crm:deal"}}, keeping.handle, testLogger())
	skips.block, keeps.block = 100*time.Millisecond, 100*time.Millisecond

	pending := func(group string) int64 {
		p, err := e.rdb.XPending(t.Context(), "gw:events:crm:deal", group).Result()
		if err != nil {
			t.Fatalf("reading the pending list: %v", err)
		}
		return p.Count
	}
	consumeUntil(t, skips, 5*time.Second, func() bool { return skipping.has(ordinary) && pending("cg:commissions") == 0 })
	consumeUntil(t, keeps, 5*time.Second, func() bool { return keeping.has(imported) && keeping.has(ordinary) })

	if skipping.has(imported) {
		t.Error("the commissions group was handed an imported event, and would accrue on a sale paid for elsewhere")
	}
	if !skipping.has(ordinary) || !keeping.has(imported) || !keeping.has(ordinary) {
		t.Errorf("deliveries: skipping saw ordinary=%v; keeping saw imported=%v ordinary=%v, want all three",
			skipping.has(ordinary), keeping.has(imported), keeping.has(ordinary))
	}
	if n := pending("cg:commissions"); n != 0 {
		t.Errorf("%d entries pending for the skipping group: a skipped event is acknowledged, not redelivered", n)
	}
}
