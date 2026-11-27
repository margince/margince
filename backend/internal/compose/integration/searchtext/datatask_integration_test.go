// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package searchtext

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// formerSearchValue is the expression the column was generated from: the whole
// body, read twice.
const formerSearchValue = `setweight(to_tsvector('simple', f_unaccent(coalesce(subject, ''))), 'A')
	|| setweight(to_tsvector('simple', f_fold_apostrophes(coalesce(subject, ''))), 'A')
	|| setweight(to_tsvector(activity_ts_config(language), f_unaccent(coalesce(subject, '') || ' ' || coalesce(body, ''))), 'B')
	|| setweight(to_tsvector('simple', f_unaccent(coalesce(body, ''))), 'C')`

// storeAsBefore puts the rows back as the release before this one stored them:
// no end, and the former search value. Only the table's owner can write that
// value past the trigger that keeps it.
func storeAsBefore(t *testing.T, rows []ids.UUID) {
	t.Helper()
	owner := integration.FreshOwnerConn(t)
	batch := &pgx.Batch{}
	batch.Queue(`ALTER TABLE activity DISABLE TRIGGER activity_search_tsv_update`)
	batch.Queue(`UPDATE activity SET body_own_end = NULL, search_tsv = `+formerSearchValue+` WHERE id = ANY($1)`, rows)
	batch.Queue(`ALTER TABLE activity ENABLE TRIGGER activity_search_tsv_update`)
	if err := owner.SendBatch(context.Background(), batch).Close(); err != nil {
		t.Fatalf("storing the rows as the former release did: %v", err)
	}
}

func TestStoredMailLosesItsQuotedTextFromSearchAcrossPagingRollbackAndReplay(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	reply := logMail(t, e, "Re: Cabinets", ownWords+quotedReply)
	plain := logMail(t, e, "Cabinets", "Can you take the walnut cabinets?")
	held := logMail(t, e, "Re: Cabinets", ownWords+quotedReply)
	subject, body := "Call notes", "They said:\n> walnut is too dark\n"
	noteRow, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Source: "manual", Subject: &subject, Body: &body})
	if err != nil {
		t.Fatalf("logging a note: %v", err)
	}
	note := ids.UUID(noteRow.Id)
	stored := []ids.UUID{reply, plain, held, note}
	storeAsBefore(t, stored)
	e.RestrictActivity(t, held)
	assertFound(t, e, reply, "walnut", true)
	if got := e.WsCount(t, outOfStep); got != len(stored) {
		t.Fatalf("%d rows hold the former search value, want all %d: the fixture is not the stored state", got, len(stored))
	}
	const editMarks = `SELECT string_agg(id::text || ':' || version::text || ':' || updated_at::text, ',' ORDER BY id) FROM activity`
	marks := e.WsScalar(t, editMarks)

	task := activities.SearchTextDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	if count, capped, err := task.Steps[0].Estimate(ctx, e.Pool, 1); err != nil || count != 1 || !capped {
		t.Fatalf("bounded estimate = %d, %t, %v", count, capped, err)
	}
	rebuild := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := rebuild(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("interrupt after the domain write")
	}
	e.RunDataTask(ctx, t, task)
	if got := e.WsCount(t, outOfStep); got != len(stored) {
		t.Fatalf("a rolled-back batch left %d rows with the former value, want %d", got, len(stored))
	}

	task.Steps[0].Batch = rebuild
	e.WsExec(t, `UPDATE data_task_run SET next_attempt_at = NULL WHERE task_id = $1`, task.ID)
	e.RunDataTask(ctx, t, task)

	assertFound(t, e, reply, "walnut", false)
	assertFound(t, e, reply, "delivery", true)
	assertFound(t, e, plain, "walnut", true)
	assertFound(t, e, note, "walnut", true)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = ANY($1) AND body_own_end IS NOT NULL`, []ids.UUID{reply, plain}); got != 2 {
		t.Errorf("%d of the 2 stored mails have an end, want both", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND body_own_end IS NULL`, note); got != 1 {
		t.Error("the note was given an end")
	}
	// A held row refuses every update, so it keeps what it had.
	assertFound(t, e, held, "walnut", true)
	if got := e.WsCount(t, outOfStep+` AND restricted_at IS NULL`); got != 0 {
		t.Errorf("%d rows still hold a search value their text does not produce", got)
	}
	if got := e.WsScalar(t, editMarks); got != marks {
		t.Error("the rebuild moved an edit version or an updated_at; no member edited these rows")
	}

	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := rebuild(ctx, tx, datatask.BatchIn{Limit: 100})
		if err != nil {
			return err
		}
		if out.Changed != 0 {
			t.Errorf("a replay wrote %d rows that were already right", out.Changed)
		}
		if _, err := rebuild(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: []byte(`invalid`)}); err == nil {
			t.Error("a malformed cursor was accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
