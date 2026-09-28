// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Opening a done line: the records a grouped line stands for, what changed on
// each, and an undo per record that carries the version the restore needs.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedEnrichedDeals writes `n` deals the rep owns and, for each, one machine
// update that set its next step — one job's work over many records.
func seedEnrichedDeals(t *testing.T, e *Env, owner ids.UUID, n int) {
	t.Helper()
	seedDealUpdates(t, e, owner, n, `{"next_step": "Call the buyer"}`)
}

// seedDealUpdates is seedEnrichedDeals with the update's after-image named.
func seedDealUpdates(t *testing.T, e *Env, owner ids.UUID, n int, after string) {
	t.Helper()
	pipeline, stage := ids.NewV7(), ids.NewV7()
	deals := make([]ids.UUID, n)
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `INSERT INTO pipeline (id, name, is_default, position)
			VALUES ($1, 'Opened line fixture', false, 92)`, pipeline); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
			VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, stage, pipeline); err != nil {
			return err
		}
		for i := range deals {
			deals[i] = ids.NewV7()
			if _, err := tx.Exec(ctx, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by)
				VALUES ($1, $2, 'Opened line deal', $3, $4, 'manual', 'human:x')`,
				deals[i], owner, pipeline, stage); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after, occurred_at)
				VALUES ('agent', 'agent:enrich', 'update', 'deal', $1,
				        '{"next_step": null}', $3::jsonb, now() - make_interval(secs => $2))`,
				deals[i], i, after); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding the enriched deals: %v", err)
	}
}

func TestAGroupedLineOpensToEveryRecordWithItsChangeAndItsOwnUndo(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	seedEnrichedDeals(t, e, e.Rep1, 3)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	svc := magic.NewService(e.Pool, nil, time.Now).WithUndoJudge(&judgeStub{undoable: true})

	receipt, err := svc.Read(ctx, &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	var line *struct{ id ids.UUID }
	for _, l := range receipt.Done {
		if l.Count != nil && *l.Count == 3 {
			line = &struct{ id ids.UUID }{ids.UUID(l.Id)}
			if l.Undo != nil {
				t.Error("a line standing for three records offers one undo for all of them")
			}
		}
	}
	if line == nil {
		t.Fatalf("no line folded the three deals; done = %+v", receipt.Done)
	}

	opened, err := svc.LineRecords(ctx, line.id, since, nil, 0)
	if err != nil {
		t.Fatalf("opening the line: %v", err)
	}
	if len(opened.Data) != 3 {
		t.Fatalf("opened %d records, want the 3 the line counted", len(opened.Data))
	}
	for _, rec := range opened.Data {
		if len(rec.Changes) != 1 || rec.Changes[0].Field != "next_step" || rec.Changes[0].After != "Call the buyer" {
			t.Errorf("changes %+v, want next_step from empty to the new value", rec.Changes)
		}
		if !rec.Undo.Undoable || rec.Undo.Version == nil {
			t.Errorf("undo %+v, want an undoable change carrying the record's version for If-Match", rec.Undo)
		}
	}

	// Paged by the cursor the first page hands out.
	limit := 2
	first, err := svc.LineRecords(ctx, line.id, since, nil, limit)
	if err != nil || len(first.Data) != 2 || !first.Page.HasMore || first.Page.NextCursor == nil {
		t.Fatalf("first page = %+v (err %v), want 2 records and a cursor", first, err)
	}
	rest, err := svc.LineRecords(ctx, line.id, since, first.Page.NextCursor, limit)
	if err != nil || len(rest.Data) != 1 || rest.Page.HasMore {
		t.Fatalf("second page = %+v (err %v), want the last record and no more", rest, err)
	}

	// An id no line in the window carries is not found, rather than an empty list.
	if _, err := svc.LineRecords(ctx, ids.NewV7(), since, nil, 0); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("opening an unknown line: %v, want ErrNotFound", err)
	}
}

// A record the reader cannot see is not in the opened line either: the regroup
// runs over the same row scope the page does.
func TestAnOpenedLineHoldsOnlyRecordsTheReaderMaySee(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	seedEnrichedDeals(t, e, e.Rep1, 2)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	svc := magic.NewService(e.Pool, nil, time.Now).WithUndoJudge(&judgeStub{undoable: true})
	receipt, err := svc.Read(ctx, &since, 20)
	if err != nil || len(receipt.Done) == 0 {
		t.Fatalf("receipt = %+v (err %v)", receipt, err)
	}
	noDeals := RepPerms
	noDeals.Objects = map[string]principal.ObjectGrant{"contact": {Read: true}}
	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, noDeals)
	if _, err := svc.LineRecords(blind, ids.UUID(receipt.Done[0].Id), since, nil, 0); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a seat that cannot read deals opened a line of deals: %v, want ErrNotFound", err)
	}
}

// A field the reader's role withholds on the deal page is withheld in the opened
// line too, with the money fields that travel with it.
func TestAnOpenedLineWithholdsWhatTheReadersRoleMasks(t *testing.T) {
	e := Setup(t)
	since := time.Now().Add(-time.Hour)
	seedDealUpdates(t, e, e.Rep1, 1,
		`{"next_step": "Call the buyer", "amount_minor": 500000, "currency": "EUR"}`)
	masked := RepPerms
	masked.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor"}}
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, masked)
	svc := magic.NewService(e.Pool, nil, time.Now)
	receipt, err := svc.Read(ctx, &since, 20)
	if err != nil || len(receipt.Done) == 0 {
		t.Fatalf("receipt = %+v (err %v)", receipt, err)
	}
	opened, err := svc.LineRecords(ctx, ids.UUID(receipt.Done[0].Id), since, nil, 0)
	if err != nil || len(opened.Data) != 1 {
		t.Fatalf("opened = %+v (err %v)", opened, err)
	}
	for _, c := range opened.Data[0].Changes {
		if c.Field == "amount_minor" || c.Field == "currency" {
			t.Errorf("the opened line shows %s to a role that masks the deal amount", c.Field)
		}
	}
	if len(opened.Data[0].Changes) != 1 {
		t.Errorf("changes %+v, want only next_step", opened.Data[0].Changes)
	}
}
