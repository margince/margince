// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// The dead-letter surface can be walked to its end.
//
// The list was bounded and honest about it — `has_more` said the page had been
// cut — and offered no way to ask for the rest. That cut falls on the OLDEST
// attempts, which on a failing subscription are the parked ones an operator
// opened the page to find: the newest attempts are the ones they already know
// about, because those are what just alerted them.
//
// This needs a database because the defect is in the keyset: whether a resumed
// walk re-serves a row, skips one, or stops early is a question about how
// Postgres orders ties, and a fake store answers whatever its author expected.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedDeliveries plants one subscription and n attempts against it.
//
// EVERY ATTEMPT SHARES ONE INSTANT, which is the case the tie-break exists for
// and the one a hand-written fixture would not think to build: a subscription
// fanning out on a single event writes its attempts in the same moment, and a
// retry storm makes that ordinary. A walk that resumed on the timestamp alone
// would re-serve or skip the whole batch.
func (e *contractVisEnv) seedDeliveries(t *testing.T, n int) (ids.UUID, []ids.UUID) {
	t.Helper()
	ctx := context.Background()
	sub := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO webhook_subscription (id, owner_id, target_url, event_types, signing_secret_ref, state)
		VALUES ($1, $2, 'https://hook.test/x', ARRAY['contact.created'], 'ref', 'active')`,
		sub, e.user); err != nil {
		t.Fatalf("seeding the subscription: %v", err)
	}
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	ordered := make([]ids.UUID, 0, n)
	for i := 0; i < n; i++ {
		id := ids.NewV7()
		if _, err := e.owner.Exec(ctx, `
			INSERT INTO webhook_delivery
			  (id, subscription_id, event_id, event_type, payload, status, attempts, created_at, updated_at)
			VALUES ($1, $2, $3, 'contact.created', '{}', 'pending', 1, $4, $4)`,
			id, sub, ids.NewV7(), at); err != nil {
			t.Fatalf("seeding delivery %d: %v", i, err)
		}
		ordered = append(ordered, id)
	}
	// Newest first is id DESC when the instant is shared, which is the order
	// the walk must reproduce across pages.
	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}
	return sub, ordered
}

func (e *contractVisEnv) asInspector() context.Context {
	return e.asHolding(map[string]principal.ObjectGrant{rbacObject: {Read: true}})
}

// EVERY ATTEMPT EXACTLY ONCE, in one order, across pages that all fall inside
// a single shared instant.
func TestTheDeliveryHistoryCanBeWalkedToItsEnd(t *testing.T) {
	e := setupContractVis(t)
	sub, want := e.seedDeliveries(t, 7)
	ctx := e.asInspector()

	var got []ids.UUID
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > len(want) {
			t.Fatal("the walk did not terminate — a cursor that does not advance pages forever")
		}
		deliveries, page, err := e.store.ListDeliveries(ctx, sub, 2, cursor)
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		for _, d := range deliveries {
			got = append(got, d.ID)
		}
		if !page.HasMore {
			if page.NextCursor != "" {
				t.Error("the last page handed back a cursor, which invites a request that " +
					"can only come back empty")
			}
			break
		}
		if page.NextCursor == "" {
			t.Fatal("a page said there was more and named no cursor — which is the defect " +
				"this walk exists to close, not a state it may reach")
		}
		cursor = page.NextCursor
	}

	if len(got) != len(want) {
		t.Fatalf("the walk saw %d attempt(s), want %d — a keyset that skips or repeats is worse "+
			"than a bound, because nothing says it happened", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("attempt %d of the walk is %s, want %s — the order across pages must be the "+
				"order within one", i, got[i], want[i])
		}
	}
}

// A CURSOR FROM SOMEWHERE ELSE IS REFUSED rather than answered.
//
// Every paged route encodes its position in the same envelope, and one naming
// only an `id` decodes cleanly here with a zero instant. On a newest-first walk
// that pages from before every row and answers an empty page — which an
// operator reads as "no more attempts", on the surface they opened precisely
// because deliveries were failing. Refusing says whose mistake it was.
func TestADeliveryCursorFromAnotherWalkIsRefused(t *testing.T) {
	e := setupContractVis(t)
	sub, _ := e.seedDeliveries(t, 3)
	ctx := e.asInspector()

	// Well-formed envelope, wrong contents: the id alone, no instant.
	foreign, err := storekit.EncodeOpaque(struct {
		ID ids.UUID `json:"id"`
	}{ID: ids.NewV7()})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := e.store.ListDeliveries(ctx, sub, 2, foreign); err == nil {
		t.Fatal("a cursor naming no position was accepted — it pages from before every row, so " +
			"the answer is an empty page that reads as a clean dead-letter queue")
	}
	if _, _, err := e.store.ListDeliveries(ctx, sub, 2, "not-a-cursor"); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}
