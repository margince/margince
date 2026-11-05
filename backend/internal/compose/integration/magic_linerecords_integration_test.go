// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Opening a done line: the records a grouped line stands for, what changed on
// each, and an undo per record that carries the version the restore needs.

import (
	"context"
	"errors"
	"strconv"
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
	seedDealActions(t, e, owner, n, "update", `{"next_step": null}`, after)
}

// seedDealActions writes `n` deals the rep owns, each with one machine action
// carrying the images named.
func seedDealActions(t *testing.T, e *Env, owner ids.UUID, n int, action, before, after string) {
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
				VALUES ('agent', 'agent:enrich', $4, 'deal', $1,
				        $5::jsonb, $3::jsonb, now() - make_interval(secs => $2))`,
				deals[i], i, after, action, before); err != nil {
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

// The arms read a row's images only for an update's sentence. A line about any
// other action still shows its images, and so does its record.
func TestAStageMoveShowsItsImagesOnTheLineAndItsRecord(t *testing.T) {
	e := Setup(t)
	seedDealActions(t, e, e.Rep1, 1, "advance_stage", `{"next_step": "Qualify"}`, `{"next_step": "Propose"}`)
	// Both instants from the database's clock, which dated the entry.
	micros, err := strconv.ParseInt(e.WsScalar(t, `SELECT (extract(epoch FROM now()) * 1e6)::bigint::text`), 10, 64)
	if err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	database := time.UnixMicro(micros)
	since := database.Add(-time.Hour)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	svc := magic.NewService(e.Pool, nil, func() time.Time { return database })

	receipt, err := svc.Read(ctx, &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	if len(receipt.Done) != 1 {
		t.Fatalf("done = %+v, want the one stage move", receipt.Done)
	}
	line := receipt.Done[0]
	if line.Before == nil || (*line.Before)["next_step"] != "Qualify" ||
		line.After == nil || (*line.After)["next_step"] != "Propose" {
		t.Errorf("line images before=%v after=%v, want next_step from Qualify to Propose", line.Before, line.After)
	}

	opened, err := svc.LineRecords(ctx, ids.UUID(line.Id), since, nil, 0)
	if err != nil {
		t.Fatalf("opening the line: %v", err)
	}
	if len(opened.Data) != 1 {
		t.Fatalf("opened %d records, want 1", len(opened.Data))
	}
	if changes := opened.Data[0].Changes; len(changes) != 1 || changes[0].Field != "next_step" ||
		changes[0].Before != "Qualify" || changes[0].After != "Propose" {
		t.Errorf("changes %+v, want next_step from Qualify to Propose", changes)
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

// The app's clock and the database's are two clocks. A line opened by an app
// a little behind the database still opens to every record it counted.
func TestALineOpensWhenTheAppClockTrailsTheDatabase(t *testing.T) {
	e := Setup(t)
	seedEnrichedDeals(t, e, e.Rep1, 3)
	// Both instants come from the database's clock, which dated the entries.
	micros, err := strconv.ParseInt(e.WsScalar(t, `SELECT (extract(epoch FROM now()) * 1e6)::bigint::text`), 10, 64)
	if err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	database := time.UnixMicro(micros)
	since := database.Add(-time.Hour)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	behind := func() time.Time { return database.Add(-2 * time.Second) }
	svc := magic.NewService(e.Pool, nil, behind).WithUndoJudge(&judgeStub{undoable: true})

	receipt, err := svc.Read(ctx, &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	var lineID ids.UUID
	for _, l := range receipt.Done {
		if l.Count != nil && *l.Count == 3 {
			lineID = ids.UUID(l.Id)
		}
	}
	if lineID.IsZero() {
		t.Fatalf("no line folded the three deals; done = %+v", receipt.Done)
	}
	opened, err := svc.LineRecords(ctx, lineID, since, nil, 0)
	if err != nil {
		t.Fatalf("opening the line with the app clock behind the database: %v", err)
	}
	if len(opened.Data) != 3 {
		t.Fatalf("opened %d records, want the 3 the line counted", len(opened.Data))
	}
}
