// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package strengthdates_test

// Dates folded before a direction-less message counted as its sender's stay
// wrong until something refolds them, because no row changes. The task does.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/strengthdates"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheDirectionlessSenderTaskRefoldsTheContactsItMoves(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	replied := clocktest.Now(t).AddDate(0, 0, -2)
	ann := e.SeedContact(t, "Ann Refold", &e.Rep1)
	e.SeedContact(t, "Bob Untouched", &e.Rep1)
	mail := ids.NewV7()
	if err := e.DB().Tx(admin, func(tx pgx.Tx) error {
		for _, stmt := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO activity (id, kind, subject, occurred_at, source_system, source_id, source, captured_by)
			  VALUES ($1, 'email', 'Re: Hallo', $2, 'gmail', $3, 'gmail:seed', 'connector:gmail')`, []any{mail, replied, mail.String()}},
			{`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, []any{mail, ann}},
			{`INSERT INTO activity_participant (activity_id, role, contact_id) VALUES ($1, 'from', $2)`, []any{mail, ann}},
			// The dates the earlier rule folded: the reply was not Ann writing.
			{`UPDATE contact SET strength_last_engaged_at = NULL, strength_last_inbound_at = NULL WHERE id = $1`, []any{ann}},
		} {
			if _, err := tx.Exec(admin, stmt.sql, stmt.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding a direction-less reply: %v", err)
	}

	task := strengthdates.DirectionlessSendersTask()
	ctx := datatask.Acting(admin, task.ID)
	changed := 0
	in := datatask.BatchIn{Limit: 200, RunID: ids.NewV7()}
	for range 1000 {
		var out datatask.BatchOut
		if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) (err error) {
			out, err = task.Steps[0].Batch(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatalf("running a batch: %v", err)
		}
		changed += out.Changed
		in.Cursor = out.Cursor
		if out.Done {
			break
		}
	}
	if changed != 1 {
		t.Errorf("the task refolded %d contacts, want Ann alone", changed)
	}
	var engaged, inbound *time.Time
	if err := e.DB().Tx(admin, func(tx pgx.Tx) error {
		return tx.QueryRow(admin, `SELECT strength_last_engaged_at, strength_last_inbound_at FROM contact WHERE id = $1`,
			ann).Scan(&engaged, &inbound)
	}); err != nil {
		t.Fatalf("reading the stored dates: %v", err)
	}
	if engaged == nil || !engaged.Equal(replied) || inbound == nil || !inbound.Equal(replied) {
		t.Errorf("Ann stores last engaged %v and last inbound %v, want the reply at %v", engaged, inbound, replied)
	}
}
