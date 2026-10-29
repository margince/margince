// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedNote writes a link-less note an own-scope reader may discover; only a
// workspace audience lets them read its content.
func (e *stagingEnv) seedNote(t *testing.T, audience string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, source, captured_by, audience) VALUES ($1, 'note', 'manual', 'system', $2)`,
		id, audience); err != nil {
		t.Fatalf("seeding a note: %v", err)
	}
	return id
}

// stageOn stages one proposal through the real writer and returns its id.
func (e *stagingEnv) stageOn(t *testing.T, kind string, deal ids.UUID, ttl *time.Duration, cites ...ids.UUID) ids.ApprovalID {
	t.Helper()
	in := StageInput{
		Kind: kind, ProposedChange: []byte(`{"stage":"won"}`), DiffHash: "batch-" + ids.NewV7().String(),
		TargetType: tableDeal, TargetID: deal, Summary: "Review the proposal", TTL: ttl,
	}
	for _, activity := range cites {
		in.Evidence = append(in.Evidence, Evidence{Snippet: "as written", SourceType: objectActivity, SourceID: activity})
	}
	id, err := e.svc.Stage(principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), "background-review"), in)
	if err != nil {
		t.Fatalf("staging %s: %v", kind, err)
	}
	return id
}

// listedIDs walks the inbox pages and returns the pending ids, sorted.
func (e *stagingEnv) listedIDs(ctx context.Context, t *testing.T, in ListInput) []string {
	t.Helper()
	var out []string
	for _, a := range pagedPending(ctx, t, e.svc, in) {
		out = append(out, a.Id.String())
	}
	slices.Sort(out)
	return out
}

// Each excluded row of one batch is undecidable for its reader for one reason
// the batch probe decides, beside rows that pass. The count and every page of
// the list must agree, and both must leave each excluded row out.
func TestTheCountAndTheListExcludeEachUndecidableRowAlike(t *testing.T) {
	e := setupStaging(t)
	seeder := newDealSeeder(t, e)
	colleague := e.secondRep(t)
	repDeal, colleagueDeal := seeder.dealOwnedBy(t, &e.rep), seeder.dealOwnedBy(t, &colleague)
	short := time.Hour
	visible := []string{
		e.stageOn(t, "advance_deal", repDeal, nil).String(),
		e.stageOn(t, "advance_deal", repDeal, nil, e.seedNote(t, "workspace")).String(),
	}
	slices.Sort(visible)
	excluded := map[string]ids.ApprovalID{
		"target out of the reader's scope": e.stageOn(t, "advance_deal", colleagueDeal, nil),
		"cites an activity it cannot read": e.stageOn(t, "advance_deal", repDeal, nil, e.seedNote(t, "workspace"), e.seedNote(t, "selected")),
		"a kind the reader muted":          e.stageOn(t, closeDateCorrectionKind, repDeal, nil),
		"expired":                          e.stageOn(t, "advance_deal", repDeal, &short),
	}
	reader := ownSeat(e.ws, e.rep)
	if _, err := e.svc.SetApprovalPreference(reader, closeDateCorrectionKind, true); err != nil {
		t.Fatal(err)
	}
	// Wall time, because staging stamps expires_at from Postgres now().
	later := time.Now().Add(2 * short)
	e.svc.now = func() time.Time { return later }

	listed := e.listedIDs(reader, t, ListInput{})
	count, err := e.svc.CountPending(reader, ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(listed, visible) || count.Count != len(visible) {
		t.Fatalf("listed %v and counted %d, want exactly %v", listed, count.Count, visible)
	}
	for reason, id := range excluded {
		if slices.Contains(listed, id.String()) {
			t.Errorf("the row that %s was listed", reason)
		}
	}

	// Requested for someone else: an all-scope reader may decide the colleague's
	// deal. The row is asked of the colleague, so the personal scope leaves it out.
	allSeat := seatFor(e.ws, e.rep, tableDeal, objectActivity)
	forColleague := excluded["target out of the reader's scope"].String()
	mine := e.listedIDs(allSeat, t, ListInput{RequestedForReader: true})
	everyone := e.listedIDs(allSeat, t, ListInput{})
	scoped, err := e.svc.CountPending(allSeat, ListInput{RequestedForReader: true})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(mine, forColleague) || !slices.Contains(everyone, forColleague) || scoped.Count != len(mine) {
		t.Fatalf("requested-for-reader listed %v (count %d), unscoped %v; the colleague's row is %v",
			mine, scoped.Count, everyone, forColleague)
	}
}

// countingTx counts the statements a probe sends.
type countingTx struct {
	pgx.Tx
	statements int
}

func (c *countingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.statements++
	return c.Tx.Query(ctx, sql, args...)
}

func (c *countingTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.statements++
	return c.Tx.QueryRow(ctx, sql, args...)
}

func (c *countingTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.statements++
	return c.Tx.Exec(ctx, sql, args...)
}

// A full batch over two owners' deals and cited notes costs one statement for
// the citations and one for the deals. Each row answers as it does alone.
func TestABatchIsDecidedInOneStatementPerSourceAndAgreesRowByRow(t *testing.T) {
	e := setupStaging(t)
	seeder := newDealSeeder(t, e)
	colleague := e.secondRep(t)
	deals := []ids.UUID{seeder.dealOwnedBy(t, &e.rep), seeder.dealOwnedBy(t, &colleague)}
	notes := []ids.UUID{e.seedNote(t, "workspace"), e.seedNote(t, "selected")}
	for i := range inboxBatch {
		e.stageOn(t, "advance_deal", deals[i%2], nil, notes[i/2%2])
	}
	reader := ownSeat(e.ws, e.rep)
	p, _ := principal.Actor(reader)
	status := statusPending
	err := e.svc.db.Tx(reader, func(tx pgx.Tx) error {
		q, args := approvalPageQuery(ListInput{Status: &status}, nil)
		batch, err := collect(reader, tx, q, args)
		if err != nil {
			return err
		}
		counted := &countingTx{Tx: tx}
		together, err := decidableRows(reader, counted, p, batch)
		if err != nil {
			return err
		}
		if len(batch) != inboxBatch || counted.statements != 2 {
			return fmt.Errorf("%d rows cost %d statements, want %d rows in 2", len(batch), counted.statements, inboxBatch)
		}
		passed := 0
		for i, a := range batch {
			alone, err := decidable(reader, tx, p, a)
			if err != nil {
				return err
			}
			if alone != together[i] {
				return fmt.Errorf("row %d answered %t alone and %t in its batch", i, alone, together[i])
			}
			if alone {
				passed++
			}
		}
		if passed != inboxBatch/4 {
			return fmt.Errorf("%d rows passed, want the %d on the reader's deal citing the readable note", passed, inboxBatch/4)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The subset probes the batch asks are spelled beside the single-row probes in
// platform/auth. Over live, archived, foreign and missing rows, for a bounded
// and an unbounded reader, each must answer what its single-row twin answers.
func TestTheSubsetProbesAnswerAsTheSingleRowProbes(t *testing.T) {
	e := setupStaging(t)
	seeder := newDealSeeder(t, e)
	colleague := e.secondRep(t)
	archived := seeder.dealOwnedBy(t, &e.rep)
	if _, err := e.owner.Exec(context.Background(), `UPDATE deal SET archived_at = now() WHERE id = $1`, archived); err != nil {
		t.Fatal(err)
	}
	deals := []ids.UUID{seeder.dealOwnedBy(t, &e.rep), seeder.dealOwnedBy(t, &colleague), archived, ids.NewV7()}
	notes := []ids.UUID{e.seedNote(t, "workspace"), e.seedNote(t, "selected"), ids.NewV7()}
	rules := map[auth.RowRule]func(context.Context, pgx.Tx, string, ids.UUID) error{
		auth.RuleVisibleLive: auth.EnsureVisibleLive, auth.RuleWritableLive: auth.EnsureWritableLive, auth.RuleWritable: auth.EnsureWritable,
	}
	readers := map[string]context.Context{"own scope": ownSeat(e.ws, e.rep), "all scope": seatFor(e.ws, e.rep, tableDeal, objectActivity)}
	for name, reader := range readers {
		err := e.svc.db.Tx(reader, func(tx pgx.Tx) error {
			for rule, single := range rules {
				subset, err := auth.PassingSubset(reader, tx, tableDeal, deals, rule)
				if err != nil {
					return err
				}
				if err := agreeRowByRow(deals, subset, func(id ids.UUID) (bool, error) { return probeAnswer(single(reader, tx, tableDeal, id)) }); err != nil {
					return fmt.Errorf("%s, rule %d: %w", name, rule, err)
				}
			}
			for live, single := range map[bool]func(context.Context, pgx.Tx, ids.UUID) error{
				false: auth.EnsureActivityContentVisible, true: auth.EnsureActivityContentVisibleLive,
			} {
				subset, err := auth.ActivityContentSubset(reader, tx, notes, live)
				if err != nil {
					return err
				}
				if err := agreeRowByRow(notes, subset, func(id ids.UUID) (bool, error) { return probeAnswer(single(reader, tx, id)) }); err != nil {
					return fmt.Errorf("%s, activity content live=%t: %w", name, live, err)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func agreeRowByRow(rowIDs []ids.UUID, subset map[ids.UUID]bool, single func(ids.UUID) (bool, error)) error {
	for i, id := range rowIDs {
		alone, err := single(id)
		if err != nil {
			return err
		}
		if alone != subset[id] {
			return fmt.Errorf("row %d answered %t alone and %t in the subset", i, alone, subset[id])
		}
	}
	return nil
}
