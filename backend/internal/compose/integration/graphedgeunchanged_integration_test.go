// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An activity event refolds an edge only when something the fold reads could
// have changed, and a refold that finds the same facts writes nothing.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedEdge is the stored row version of one interaction edge: xmin changes on
// every rewrite, so an equal value proves nothing was written.
type storedEdge struct {
	xmin  string
	total int
}

type graphSkipEnv struct{ e *Env }

func (v graphSkipEnv) inTx(t *testing.T, run func(tx pgx.Tx) error) {
	t.Helper()
	if err := database.WithWorkspaceTx(v.e.Admin(), v.e.Pool, run); err != nil {
		t.Fatal(err)
	}
}

func (v graphSkipEnv) contact(t *testing.T, name string) ids.ContactID {
	t.Helper()
	id := ids.New[ids.ContactKind]()
	v.e.WsExec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by, visibility)
		VALUES ($1, $2, $3, 'manual', 'human:test', 'workspace')`, id, name, v.e.Rep1)
	return id
}

// interaction writes an inbound mail from the contact to Rep1 at the given time.
func (v graphSkipEnv) interaction(t *testing.T, user ids.UUID, contact ids.ContactID, at time.Time, direction string) ids.UUID {
	t.Helper()
	var activity ids.UUID
	userRole, contactRole := "to", "from"
	if direction == "outbound" {
		userRole, contactRole = "from", "to"
	}
	v.inTx(t, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `INSERT INTO activity (kind, subject, direction, occurred_at, source, captured_by)
			VALUES ('email', 'Betreff', $1, $2, 'manual', 'human:test') RETURNING id`, direction, at).Scan(&activity); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, $3)`,
			activity, user, userRole); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, $3)`,
			activity, contact, contactRole)
		return err
	})
	return activity
}

func (v graphSkipEnv) recompute(t *testing.T, activities ...ids.UUID) {
	t.Helper()
	v.inTx(t, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(v.e.Admin(), tx, activities)
	})
}

func (v graphSkipEnv) edge(t *testing.T, user ids.UUID, contact ids.ContactID) (storedEdge, bool) {
	t.Helper()
	var row storedEdge
	found := true
	v.inTx(t, func(tx pgx.Tx) error {
		err := tx.QueryRow(context.Background(), `SELECT xmin::text, count_total FROM graph_interaction_edge
			WHERE user_id = $1 AND contact_id = $2`, user, contact).Scan(&row.xmin, &row.total)
		if err == pgx.ErrNoRows {
			found = false
			return nil
		}
		return err
	})
	return row, found
}

func (v graphSkipEnv) snapshot(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	v.inTx(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT user_id, contact_id, count_total FROM graph_interaction_edge`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var user, contact ids.UUID
			var total int
			if err := rows.Scan(&user, &contact, &total); err != nil {
				return err
			}
			out[user.String()+"/"+contact.String()] = total
		}
		return rows.Err()
	})
	return out
}

func (v graphSkipEnv) event(t *testing.T, eventType string, activity ids.UUID, payload string) kevents.Envelope {
	t.Helper()
	env := kevents.Envelope{
		EventID: ids.NewV7(), Type: eventType, Version: 1, OccurredAt: clocktest.Now(t),
		Entity: kevents.EntityRef{Type: "activity", ID: activity},
		Trace:  kevents.Trace{CorrelationID: ids.NewV7()},
	}
	if payload != "" {
		env.Payload = []byte(payload)
	}
	return env
}

func (v graphSkipEnv) handle(t *testing.T, env kevents.Envelope) {
	t.Helper()
	if err := search.NewGraphEdgeGen(search.NewStore(v.e.DB())).HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("%s: %v", env.Type, err)
	}
}

func TestAnUpdateTheFoldCannotSeeLeavesTheEdgeAlone(t *testing.T) {
	v := graphSkipEnv{Setup(t)}
	contact := v.contact(t, "Classified Contact")
	activity := v.interaction(t, v.e.Rep1, contact, clocktest.Now(t).AddDate(0, 0, -2), "inbound")
	v.recompute(t, activity)
	if settled, ok := v.edge(t, v.e.Rep1, contact); !ok || settled.total != 1 {
		t.Fatalf("setup edge=%+v found=%t", settled, ok)
	}

	// A wrong stored edge shows whether an event refolded: only a refold repairs it.
	v.e.WsExec(t, `UPDATE graph_interaction_edge SET count_total = 99 WHERE contact_id = $1`, contact)
	stale, _ := v.edge(t, v.e.Rep1, contact)

	for _, payload := range []string{
		`{"changed_fields":{"body":true}}`,
		`{"changed_fields":{"assignee_id":"` + v.e.Rep2.String() + `","automatic_task_repaired":true}}`,
		`{"changed_fields":{"agenda_changed":true}}`,
	} {
		v.handle(t, v.event(t, "activity.updated", activity, payload))
		if got, _ := v.edge(t, v.e.Rep1, contact); got != stale {
			t.Fatalf("%s refolded the edge: %+v, was %+v", payload, got, stale)
		}
	}

	v.handle(t, v.event(t, "activity.updated", activity, ""))
	if got, _ := v.edge(t, v.e.Rep1, contact); got.total != 1 {
		t.Fatalf("an update without a field list left the edge at %d, want a refold to 1", got.total)
	}
}

func TestARefoldThatFindsTheSameFactsWritesNothing(t *testing.T) {
	v := graphSkipEnv{Setup(t)}
	contact := v.contact(t, "Steady Contact")
	activity := v.interaction(t, v.e.Rep1, contact, clocktest.Now(t).AddDate(0, 0, -2), "inbound")
	v.recompute(t, activity)
	before, _ := v.edge(t, v.e.Rep1, contact)

	for _, env := range []kevents.Envelope{
		v.event(t, "activity.updated", activity, `{"changed_fields":{"occurred_at":"2026-01-01T00:00:00Z"}}`),
		v.event(t, "activity.captured", activity, ""),
		v.event(t, "activity.updated", activity, ""),
	} {
		v.handle(t, env)
		if got, _ := v.edge(t, v.e.Rep1, contact); got != before {
			t.Fatalf("%s rewrote an unchanged edge: %+v, was %+v", env.Type, got, before)
		}
	}
	batch := []kevents.Envelope{v.event(t, "activity.updated", activity, ""), v.event(t, "activity.updated", activity, "")}
	if err := search.NewGraphEdgeGen(search.NewStore(v.e.DB())).HandleBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.edge(t, v.e.Rep1, contact); got != before {
		t.Fatalf("a batch rewrote an unchanged edge: %+v, was %+v", got, before)
	}
}

func TestARealChangeStillUpdatesTheEdgeAndTheRebuildAgrees(t *testing.T) {
	v := graphSkipEnv{Setup(t)}
	now := clocktest.Now(t)
	contact := v.contact(t, "Moving Contact")
	other := v.contact(t, "Other Contact")
	first := v.interaction(t, v.e.Rep1, contact, now.AddDate(0, 0, -3), "inbound")
	second := v.interaction(t, v.e.Rep1, other, now.AddDate(0, 0, -4), "inbound")
	kept := v.interaction(t, v.e.Rep2, other, now.AddDate(0, 0, -5), "outbound")
	v.recompute(t, first, second, kept)
	before, _ := v.edge(t, v.e.Rep1, contact)

	v.e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'cc')`, first, v.e.Rep2)
	v.handle(t, v.event(t, "activity.updated", first, `{"changed_fields":{"body":true,"subject":"x"}}`))
	if _, found := v.edge(t, v.e.Rep2, contact); found {
		t.Fatal("a body-only update folded a participant the fold had not been told about")
	}
	relinked := `{"changed_fields":{"relinked":{"entity_type":"contact","entity_id":"` + contact.String() + `"}}}`
	v.handle(t, v.event(t, "activity.updated", first, relinked))
	if _, found := v.edge(t, v.e.Rep2, contact); !found {
		t.Fatal("a new participant did not get an edge")
	}

	v.e.WsExec(t, `UPDATE activity SET occurred_at = $2 WHERE id = $1`, first, now.AddDate(0, 0, -1))
	v.handle(t, v.event(t, "activity.updated", first, `{"changed_fields":{"occurred_at":"x"}}`))
	if got, _ := v.edge(t, v.e.Rep1, contact); got.xmin == before.xmin {
		t.Fatal("a changed occurred_at left the edge unwritten")
	}

	v.e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, second)
	v.handle(t, v.event(t, "activity.archived", second, ""))
	if _, found := v.edge(t, v.e.Rep1, other); found {
		t.Fatal("an archived last interaction kept its edge")
	}

	v.e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, first)
	v.handle(t, v.event(t, "retention.applied", first, ""))
	if _, found := v.edge(t, v.e.Rep1, contact); found {
		t.Fatal("retention left an edge without evidence")
	}

	incremental := v.snapshot(t)
	if len(incremental) == 0 {
		t.Fatal("the surviving interaction has no edge, so the comparison would pass vacuously")
	}
	v.inTx(t, func(tx pgx.Tx) error { return search.RebuildEdges(v.e.Admin(), tx) })
	rebuilt := v.snapshot(t)
	if len(rebuilt) != len(incremental) {
		t.Fatalf("the reconcile found a difference after mixed events:\n  incremental %v\n  rebuilt     %v", incremental, rebuilt)
	}
	for key, total := range incremental {
		if rebuilt[key] != total {
			t.Fatalf("the reconcile found a difference after mixed events:\n  incremental %v\n  rebuilt     %v", incremental, rebuilt)
		}
	}
}

func TestAChangeThatOnlyMovesTheSentCountRewritesTheEdge(t *testing.T) {
	v := graphSkipEnv{Setup(t)}
	contact := v.contact(t, "Sent Count Contact")
	activity := v.interaction(t, v.e.Rep1, contact, clocktest.Now(t).AddDate(0, 0, -2), "outbound")
	v.e.WsExec(t, `UPDATE activity_participant SET role = 'cc' WHERE activity_id = $1 AND user_id = $2`, activity, v.e.Rep1)
	v.recompute(t, activity)
	sent := func() int {
		return v.e.WsCount(t, `SELECT coalesce(sent_count_90d, -1) FROM graph_interaction_edge WHERE user_id = $1 AND contact_id = $2`, v.e.Rep1, contact)
	}
	before, _ := v.edge(t, v.e.Rep1, contact)
	if sent() != 0 {
		t.Fatalf("a copied colleague has sent %d messages, want 0", sent())
	}

	v.e.WsExec(t, `UPDATE activity_participant SET role = 'from' WHERE activity_id = $1 AND user_id = $2`, activity, v.e.Rep1)
	v.handle(t, v.event(t, "activity.updated", activity, `{"changed_fields":{"relinked":{"entity_type":"contact","entity_id":"`+contact.String()+`"}}}`))
	after, _ := v.edge(t, v.e.Rep1, contact)
	if sent() != 1 || after.xmin == before.xmin {
		t.Fatalf("sent=%d rewritten=%t, want the edge rewritten with one sent message", sent(), after.xmin != before.xmin)
	}
}
