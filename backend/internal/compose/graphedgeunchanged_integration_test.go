// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An activity event refolds an edge only when something the fold reads could
// have changed, and a refold that finds the same facts writes nothing.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedEdge is the stored row version of one interaction edge: xmin changes on
// every rewrite, so an equal value proves nothing was written.
type storedEdge struct {
	xmin     string
	computed time.Time
	total    int
}

func (v edgeEnv) storedEdge(t *testing.T, user ids.UUID, contact ids.ContactID) (storedEdge, bool) {
	t.Helper()
	var row storedEdge
	found := true
	inWorkspaceTx(t, v.e, func(tx pgx.Tx) error {
		err := tx.QueryRow(context.Background(), `SELECT xmin::text, computed_at, count_total FROM graph_interaction_edge
			WHERE user_id = $1 AND contact_id = $2`, user, contact).Scan(&row.xmin, &row.computed, &row.total)
		if err == pgx.ErrNoRows {
			found = false
			return nil
		}
		return err
	})
	return row, found
}

func updatedEvent(v edgeEnv, activity ids.UUID, payload string) kevents.Envelope {
	env := envelopeFor(v.e.WS, "activity.updated", "activity", activity)
	if payload != "" {
		env.Payload = []byte(payload)
	}
	return env
}

func (v edgeEnv) handle(t *testing.T, env kevents.Envelope) {
	t.Helper()
	if err := search.NewGraphEdgeGen(search.NewStore(v.e.DB())).HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("%s: %v", env.Type, err)
	}
}

func TestAnUpdateTheFoldCannotSeeLeavesTheEdgeAlone(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	contact := v.contact(t, "Classified Contact")
	activity := v.interaction(t, v.e.Rep1, contact, time.Now().UTC().AddDate(0, 0, -2), "inbound", "from")
	v.recompute(t, activity)
	settled, ok := v.storedEdge(t, v.e.Rep1, contact)
	if !ok || settled.total != 1 {
		t.Fatalf("setup edge=%+v found=%t", settled, ok)
	}

	// A wrong stored edge shows whether an event refolded: only a refold repairs it.
	v.e.WsExec(t, `UPDATE graph_interaction_edge SET count_total = 99 WHERE contact_id = $1`, contact)
	stale, _ := v.storedEdge(t, v.e.Rep1, contact)

	for _, payload := range []string{
		`{"changed_fields":{"body":true}}`,
		`{"changed_fields":{"assignee_id":"` + v.e.Rep2.String() + `","automatic_task_repaired":true}}`,
		`{"changed_fields":{"agenda_changed":true}}`,
	} {
		v.handle(t, updatedEvent(v, activity, payload))
		if got, _ := v.storedEdge(t, v.e.Rep1, contact); got != stale {
			t.Fatalf("%s refolded the edge: %+v, was %+v", payload, got, stale)
		}
	}

	v.handle(t, updatedEvent(v, activity, ""))
	if got, _ := v.storedEdge(t, v.e.Rep1, contact); got.total != 1 {
		t.Fatalf("an update without a field list left the edge at %d, want a refold to 1", got.total)
	}
}

func TestARefoldThatFindsTheSameFactsWritesNothing(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	contact := v.contact(t, "Steady Contact")
	activity := v.interaction(t, v.e.Rep1, contact, time.Now().UTC().AddDate(0, 0, -2), "inbound", "from")
	v.recompute(t, activity)
	before, _ := v.storedEdge(t, v.e.Rep1, contact)

	for _, env := range []kevents.Envelope{
		updatedEvent(v, activity, `{"changed_fields":{"occurred_at":"2026-01-01T00:00:00Z"}}`),
		envelopeFor(v.e.WS, "activity.captured", "activity", activity),
		updatedEvent(v, activity, ""),
	} {
		v.handle(t, env)
		if got, _ := v.storedEdge(t, v.e.Rep1, contact); got != before {
			t.Fatalf("%s rewrote an unchanged edge: %+v, was %+v", env.Type, got, before)
		}
	}
	if err := search.NewGraphEdgeGen(search.NewStore(v.e.DB())).HandleBatch(context.Background(),
		[]kevents.Envelope{updatedEvent(v, activity, ""), updatedEvent(v, activity, "")}); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.storedEdge(t, v.e.Rep1, contact); got != before {
		t.Fatalf("a batch rewrote an unchanged edge: %+v, was %+v", got, before)
	}
}

func TestARealChangeStillUpdatesTheEdgeAndTheRebuildAgrees(t *testing.T) {
	v := edgeEnv{integration.Setup(t)}
	now := time.Now().UTC()
	contact := v.contact(t, "Moving Contact")
	other := v.contact(t, "Other Contact")
	first := v.interaction(t, v.e.Rep1, contact, now.AddDate(0, 0, -3), "inbound", "from")
	second := v.interaction(t, v.e.Rep1, other, now.AddDate(0, 0, -4), "inbound", "from")
	kept := v.interaction(t, v.e.Rep2, other, now.AddDate(0, 0, -5), "outbound", "to")
	v.recompute(t, first, second, kept)
	before, _ := v.storedEdge(t, v.e.Rep1, contact)

	v.copyColleague(t, first, v.e.Rep2)
	v.handle(t, updatedEvent(v, first, `{"changed_fields":{"body":true,"subject":"x"}}`))
	if _, found := v.storedEdge(t, v.e.Rep2, contact); found {
		t.Fatal("a body-only update folded a participant the fold had not been told about")
	}
	v.handle(t, updatedEvent(v, first, `{"changed_fields":{"relinked":{"entity_type":"contact","entity_id":"`+contact.String()+`"}}}`))
	if _, found := v.storedEdge(t, v.e.Rep2, contact); !found {
		t.Fatal("a new participant did not get an edge")
	}

	v.e.WsExec(t, `UPDATE activity SET occurred_at = $2 WHERE id = $1`, first, now.AddDate(0, 0, -1))
	v.handle(t, updatedEvent(v, first, `{"changed_fields":{"occurred_at":"x"}}`))
	if got, _ := v.storedEdge(t, v.e.Rep1, contact); got.xmin == before.xmin {
		t.Fatal("a changed occurred_at left the edge unwritten")
	}

	v.e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, second)
	v.handle(t, envelopeFor(v.e.WS, "activity.archived", "activity", second))
	if _, found := v.storedEdge(t, v.e.Rep1, other); found {
		t.Fatal("an archived last interaction kept its edge")
	}

	v.e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, first)
	v.handle(t, envelopeFor(v.e.WS, "retention.applied", "activity", first))
	if _, found := v.storedEdge(t, v.e.Rep1, contact); found {
		t.Fatal("retention left an edge without evidence")
	}

	incremental := v.snapshot(t)
	if len(incremental) == 0 {
		t.Fatal("the surviving interaction has no edge, so the comparison would pass vacuously")
	}
	inWorkspaceTx(t, v.e, func(tx pgx.Tx) error { return search.RebuildEdges(v.e.Admin(), tx) })
	if rebuilt := v.snapshot(t); !equalSnapshots(incremental, rebuilt) {
		t.Fatalf("the reconcile found a difference after mixed events:\n  incremental %v\n  rebuilt     %v", incremental, rebuilt)
	}
}
