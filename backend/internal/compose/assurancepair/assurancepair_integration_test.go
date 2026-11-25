// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assurancepair_test

// Two open deals of one account that are one buying motion. The deals and the
// meeting they share are seeded as the sibling input-check suites seed them.
// The check runs as the product wires it. The answer and the merge go through
// the stores' own entry points.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/assurancepair"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

type pairEnv struct {
	*integration.Env
	// earlier and later are the pair; the finding is filed on later.
	earlier, later ids.UUID
	company        ids.UUID
	pipeline       ids.PipelineID
	stage          ids.StageID
}

// run is the input check as the product wires it, run as the system the
// nightly job binds.
func (e *pairEnv) run(t *testing.T) {
	t.Helper()
	store := assurance.NewStore(compose.InstallationDB(e.Pool))
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:assurance")
	if _, err := compose.AssuranceScanner(e.Pool, store).Scan(ctx, clocktest.Now(t)); err != nil {
		t.Fatalf("the check: %v", err)
	}
}

// setupPair seeds an account with two open deals that share one meeting and
// close 30 days apart.
func setupPair(t *testing.T) *pairEnv {
	t.Helper()
	e := &pairEnv{Env: integration.Setup(t)}
	e.pipeline, e.stage, _ = integration.DealFixture(t, e.Env)
	owner := integration.OwnerConn(t)
	e.company = integration.SeedIDRow(t, owner, `
		INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'One Account', 'manual', 'test')`)
	e.earlier = e.seedDeal(t, owner, "Pair earlier", 40)
	e.later = e.seedDeal(t, owner, "Pair later", 70)
	meeting := integration.SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Kick-off', now() - interval '3 days', 'manual', 'test')`)
	integration.LinkActivity(t, owner, meeting, "deal", e.earlier)
	integration.LinkActivity(t, owner, meeting, "deal", e.later)
	return e
}

func (e *pairEnv) seedDeal(t *testing.T, owner *pgx.Conn, name string, closeInDays int) ids.UUID {
	t.Helper()
	id := integration.SeedIDRow(t, owner, `
		INSERT INTO deal (id, pipeline_id, stage_id, name, company_id, owner_id, status, source, captured_by,
		                  amount_minor, currency, expected_close_date, win_probability_override, win_probability_set_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'open', 'manual', 'test', 1000000, 'EUR', now()::date + $7::int, 10, 'human')`,
		e.pipeline, e.stage, name, e.company, e.Rep1, closeInDays)
	// Created later, whatever the clock resolution: the pair's order is created_at.
	if _, err := owner.Exec(context.Background(),
		`UPDATE deal SET created_at = now() + ($2::int * interval '1 second') WHERE id = $1`, id, closeInDays); err != nil {
		t.Fatalf("ordering the pair: %v", err)
	}
	return id
}

// pairFinding is the stored finding for the pair, whatever its status.
func (e *pairEnv) pairFinding(t *testing.T) (id ids.UUID, status string, found bool) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT id, status FROM assurance_exception WHERE type = $1`,
			assurance.TypeDuplicateMotion).Scan(&id, &status)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, "", false
	}
	if err != nil {
		t.Fatalf("reading the pair finding: %v", err)
	}
	return id, status, true
}

func (e *pairEnv) pairFindingCount(t *testing.T) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM assurance_exception WHERE type = $1`, assurance.TypeDuplicateMotion)
}

func (e *pairEnv) setClose(t *testing.T, deal ids.UUID, days int) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE deal SET expected_close_date = now()::date + $2::int WHERE id = $1`, deal, days); err != nil {
		t.Fatalf("moving a close date: %v", err)
	}
}

func TestAPairSharingAMeetingRaisesOneFindingOnTheLaterDeal(t *testing.T) {
	e := setupPair(t)
	for range 2 {
		e.run(t)
	}
	if n := e.pairFindingCount(t); n != 1 {
		t.Fatalf("%d pair findings after two runs, want 1", n)
	}
	var subject ids.UUID
	var observed []byte
	if err := e.Pool.QueryRow(e.Admin(), `SELECT subject_id, observed FROM assurance_exception WHERE type = $1`,
		assurance.TypeDuplicateMotion).Scan(&subject, &observed); err != nil {
		t.Fatal(err)
	}
	var slots map[string]any
	if err := json.Unmarshal(observed, &slots); err != nil {
		t.Fatal(err)
	}
	if subject != e.later || slots[assurance.SlotOtherDealID] != e.earlier.String() {
		t.Fatalf("filed on %s naming %v, want the later deal %s naming %s", subject, slots[assurance.SlotOtherDealID], e.later, e.earlier)
	}
}

func TestPairsThatAreNotOneMotionRaiseNothing(t *testing.T) {
	for name, change := range map[string]func(*pairEnv, *pgx.Conn){
		"61 days apart": func(e *pairEnv, _ *pgx.Conn) { e.setClose(t, e.later, 101) },
		"another pipeline": func(e *pairEnv, owner *pgx.Conn) {
			other := integration.SeedIDRow(t, owner, `INSERT INTO pipeline (id, name) VALUES ($1, 'Other')`)
			stage := integration.SeedIDRow(t, owner, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Open', 0)`, other)
			execSeed(t, owner, `UPDATE deal SET pipeline_id = $2, stage_id = $3 WHERE id = $1`, e.later, other, stage)
		},
		"another account": func(e *pairEnv, owner *pgx.Conn) {
			other := integration.SeedIDRow(t, owner, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Another', 'manual', 'test')`)
			execSeed(t, owner, `UPDATE deal SET company_id = $2 WHERE id = $1`, e.later, other)
		},
		"no close date": func(e *pairEnv, owner *pgx.Conn) {
			execSeed(t, owner, `UPDATE deal SET expected_close_date = NULL WHERE id = $1`, e.later)
		},
		"one deal is archived": func(e *pairEnv, owner *pgx.Conn) {
			execSeed(t, owner, `UPDATE deal SET archived_at = now() WHERE id = $1`, e.later)
		},
		"one deal is lost": func(e *pairEnv, owner *pgx.Conn) {
			execSeed(t, owner, `UPDATE deal SET status = 'lost', lost_reason = 'gone', closed_at = now(),
				fx_rate_to_base = 1 WHERE id = $1`, e.later)
		},
		"no shared evidence": func(e *pairEnv, owner *pgx.Conn) {
			execSeed(t, owner, `DELETE FROM activity_link WHERE deal_id = $1`, e.later)
		},
		"a shared note, which is neither mail nor a meeting": func(e *pairEnv, owner *pgx.Conn) {
			execSeed(t, owner, `UPDATE activity SET kind = 'note'`)
		},
		"a shared meeting only its participants may read": func(e *pairEnv, owner *pgx.Conn) {
			execSeed(t, owner, `UPDATE activity SET audience = 'participants'`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupPair(t)
			change(e, integration.OwnerConn(t))
			e.run(t)
			if n := e.pairFindingCount(t); n != 0 {
				t.Fatalf("%d pair findings, want none", n)
			}
		})
	}
}

func execSeed(t *testing.T, conn *pgx.Conn, sql string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}

func TestASharedStakeholderAloneRaisesTheFinding(t *testing.T) {
	e := setupPair(t)
	owner := integration.OwnerConn(t)
	execSeed(t, owner, `DELETE FROM activity_link WHERE deal_id = $1`, e.later)
	contact := integration.SeedIDRow(t, owner, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Both', 'manual', 'test')`)
	for _, deal := range []ids.UUID{e.earlier, e.later} {
		execSeed(t, owner, `INSERT INTO relationship (kind, contact_id, deal_id, source, captured_by)
			VALUES ('deal_stakeholder', $1, $2, 'manual', 'test')`, contact, deal)
	}
	e.run(t)
	if n := e.pairFindingCount(t); n != 1 {
		t.Fatalf("%d pair findings, want 1", n)
	}
}

// The finding closes when the pair stops matching, and an answer of "separate"
// keeps it closed when the pair matches again.
func TestSeparateKeepsAPairClosedAndAClearedPairReopens(t *testing.T) {
	e := setupPair(t)
	e.run(t)
	e.setClose(t, e.later, 130)
	e.run(t)
	if _, status, _ := e.pairFinding(t); status != assurance.ExceptionConditionCleared {
		t.Fatalf("status = %q after the dates moved 90 days apart, want condition_cleared", status)
	}
	e.setClose(t, e.later, 70)
	e.run(t)
	id, status, _ := e.pairFinding(t)
	if status != "open" {
		t.Fatalf("status = %q after the pair matched again, want open", status)
	}
	if err := assurance.NewStore(compose.InstallationDB(e.Pool)).Resolve(e.Admin(), id,
		assurance.Resolution{Outcome: assurance.OutcomeSeparate}, clocktest.Now(t)); err != nil {
		t.Fatalf("answering separate: %v", err)
	}
	e.setClose(t, e.later, 130)
	e.run(t)
	e.setClose(t, e.later, 70)
	for range 2 {
		e.run(t)
	}
	if _, status, _ := e.pairFinding(t); status != "resolved" || e.pairFindingCount(t) != 1 {
		t.Fatalf("status = %q with %d findings, want the one answered finding left alone", status, e.pairFindingCount(t))
	}
}

func TestSeparateIsRefusedOnAnyOtherFinding(t *testing.T) {
	e := setupPair(t)
	e.seedDeal(t, integration.OwnerConn(t), "Past due", -10)
	e.run(t)
	var other ids.UUID
	if err := e.Pool.QueryRow(e.Admin(), `SELECT id FROM assurance_exception WHERE type <> $1 AND status = 'open' LIMIT 1`,
		assurance.TypeDuplicateMotion).Scan(&other); err != nil {
		t.Fatalf("the fixture needs another open finding: %v", err)
	}
	err := assurance.NewStore(compose.InstallationDB(e.Pool)).Resolve(e.Admin(), other,
		assurance.Resolution{Outcome: assurance.OutcomeSeparate}, clocktest.Now(t))
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Field != "outcome" {
		t.Fatalf("separate on another finding answered %v, want a refusal naming outcome", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM assurance_resolution WHERE outcome = 'separate'`); n != 0 {
		t.Fatalf("%d separate answers stored on a refused answer", n)
	}
}

// Merging the later deal into the earlier one retires the later deal, so the
// finding filed on it closes as departed.
func TestMergingThePairClosesItsFinding(t *testing.T) {
	e := setupPair(t)
	e.run(t)
	if _, err := e.Deals.MergeDeal(e.Admin(), ids.From[ids.DealKind](e.later), ids.From[ids.DealKind](e.earlier), nil); err != nil {
		t.Fatalf("merging the pair: %v", err)
	}
	e.run(t)
	if _, status, _ := e.pairFinding(t); status != assurance.ExceptionSubjectDeparted {
		t.Fatalf("status = %q after the merge, want subject_departed", status)
	}
}

// A reader who may not see the second deal gets the finding and no id for it.
func TestAReaderWithoutTheOtherDealIsNotNamedIt(t *testing.T) {
	e := setupPair(t)
	e.run(t)
	read := func(reader context.Context) assurance.Exception {
		var pair assurance.Exception
		if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
			var found []assurance.Exception
			err := tx.QueryRow(reader, `SELECT id, type, subject_kind, subject_id, observed FROM assurance_exception WHERE type = $1`,
				assurance.TypeDuplicateMotion).Scan(&pair.ID, &pair.Type, &pair.SubjectKind, &pair.SubjectID, &pair.Observed)
			if err != nil {
				return err
			}
			found, err = assurancepair.WithholdHidden(reader, tx, []assurance.Exception{pair})
			pair = found[0]
			return err
		}); err != nil {
			t.Fatalf("narrowing the finding: %v", err)
		}
		return pair
	}
	named := func(f assurance.Exception) bool {
		var slots map[string]any
		if err := json.Unmarshal(f.Observed, &slots); err != nil {
			t.Fatal(err)
		}
		_, has := slots[assurance.SlotOtherDealID]
		return has
	}
	if !named(read(e.Admin())) {
		t.Fatal("a reader of both deals was not told the second deal")
	}
	blind := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"forecast": {Read: true}},
	})
	if named(read(blind)) {
		t.Fatal("a reader with no access to deals was handed the second deal's id")
	}
}

// rescan is the pass over one changed deal, which the freshness marker queues.
func (e *pairEnv) rescan(t *testing.T, deal ids.UUID) {
	t.Helper()
	execSeed(t, integration.OwnerConn(t), `INSERT INTO assurance_input_change (subject_kind, subject_id, generation)
		VALUES ('deal', $1, 1) ON CONFLICT (subject_kind, subject_id) DO UPDATE SET generation = assurance_input_change.generation + 1`, deal)
	store := assurance.NewStore(compose.InstallationDB(e.Pool))
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:assurance")
	if _, err := compose.AssuranceScanner(e.Pool, store).ScanDeal(ctx, clocktest.Now(t), deal); err != nil {
		t.Fatalf("the pass over one deal: %v", err)
	}
}

// A pass over one changed deal reconciles every pair that includes it.
func TestAPassOverOneDealReconcilesItsPairs(t *testing.T) {
	e := setupPair(t)
	owner := integration.OwnerConn(t)
	e.rescan(t, e.earlier)
	if _, status, found := e.pairFinding(t); !found || status != "open" {
		t.Fatalf("a new matching pair: found=%v status=%q, want it raised on its own pass", found, status)
	}
	e.setClose(t, e.earlier, 200)
	e.rescan(t, e.earlier)
	if _, status, _ := e.pairFinding(t); status != assurance.ExceptionConditionCleared {
		t.Fatalf("status = %q after the earlier deal's date left the window, want condition_cleared", status)
	}
	e.setClose(t, e.earlier, 40)
	e.rescan(t, e.later)
	if _, status, _ := e.pairFinding(t); status != "open" {
		t.Fatalf("status = %q after the date came back, want open", status)
	}
	other := integration.SeedIDRow(t, owner, `INSERT INTO pipeline (id, name) VALUES ($1, 'Other')`)
	stage := integration.SeedIDRow(t, owner, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Open', 0)`, other)
	execSeed(t, owner, `UPDATE deal SET pipeline_id = $2, stage_id = $3 WHERE id = $1`, e.earlier, other, stage)
	e.rescan(t, e.earlier)
	if _, status, _ := e.pairFinding(t); status != assurance.ExceptionConditionCleared {
		t.Fatalf("status = %q after the deal changed pipeline, want condition_cleared", status)
	}
}

// A pass over one deal closes only the pair findings that include it, not
// another pair filed on the same deal.
func TestAPassOverOneDealLeavesAnotherPairFiledOnTheSameDealOpen(t *testing.T) {
	e := setupPair(t)
	owner := integration.OwnerConn(t)
	third := e.seedDeal(t, owner, "Pair third", 55)
	meeting := integration.SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Second', now() - interval '2 days', 'manual', 'test')`)
	integration.LinkActivity(t, owner, meeting, "deal", third)
	integration.LinkActivity(t, owner, meeting, "deal", e.later)
	e.run(t)
	if n := e.WsCount(t, `SELECT count(*) FROM assurance_exception WHERE type = $1 AND status = 'open' AND subject_id = $2`,
		assurance.TypeDuplicateMotion, e.later); n != 2 {
		t.Fatalf("%d open pair findings on the later deal, want 2", n)
	}
	execSeed(t, owner, `DELETE FROM activity_link WHERE deal_id = $1`, e.earlier)
	e.rescan(t, e.earlier)
	open := e.WsCount(t, `SELECT count(*) FROM assurance_exception WHERE type = $1 AND status = 'open'
		AND observed ->> 'other_deal_id' = $2`, assurance.TypeDuplicateMotion, third.String())
	cleared := e.WsCount(t, `SELECT count(*) FROM assurance_exception WHERE type = $1 AND status = $3
		AND observed ->> 'other_deal_id' = $2`, assurance.TypeDuplicateMotion, e.earlier.String(), assurance.ExceptionConditionCleared)
	if open != 1 || cleared != 1 {
		t.Fatalf("open pair with the third deal = %d, cleared pair with the earlier deal = %d, want 1 and 1", open, cleared)
	}
}

// Every answer on a pair finding needs write authority over both deals, as the
// list offers it. A reader who may not write the second deal is refused all.
func TestEveryAnswerOnAPairNeedsBothDeals(t *testing.T) {
	e := setupPair(t)
	e.run(t)
	execSeed(t, integration.OwnerConn(t), `UPDATE deal SET owner_id = $2 WHERE id = $1`, e.earlier, e.AdminUser)
	reader := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"forecast": {Read: true, Update: true}, "deal": {Read: true, Update: true},
		},
	})
	id, _, _ := e.pairFinding(t)
	now := clocktest.Now(t)
	later := now.Add(24 * time.Hour)
	for _, answer := range []assurance.Resolution{
		{Outcome: assurance.OutcomeFixedRecord},
		{Outcome: assurance.OutcomeAddedEvidence},
		{Outcome: assurance.OutcomeValueCorrect, Reason: "checked"},
		{Outcome: assurance.OutcomeNotRelevant, Reason: "checked"},
		{Outcome: assurance.OutcomeRemindLater, RemindAt: &later},
		{Outcome: assurance.OutcomeReassign},
		{Outcome: assurance.OutcomeSeparate},
	} {
		if err := assurance.NewStore(compose.InstallationDB(e.Pool)).Resolve(reader, id, answer, now); err == nil {
			t.Errorf("%s: a reader who may write only the later deal was allowed it", answer.Outcome)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM assurance_resolution`); n != 0 {
		t.Fatalf("%d answers stored from refused calls", n)
	}
	if _, status, _ := e.pairFinding(t); status != "open" {
		t.Fatalf("status = %q, want open", status)
	}
}

// Answering separate needs write authority over both deals. A reader who may
// not write the second deal is neither offered the answer nor allowed it.
func TestSeparateNeedsAuthorityOverBothDeals(t *testing.T) {
	e := setupPair(t)
	e.run(t)
	execSeed(t, integration.OwnerConn(t), `UPDATE deal SET owner_id = $2 WHERE id = $1`, e.earlier, e.AdminUser)
	reader := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"forecast": {Read: true, Update: true}, "deal": {Read: true, Update: true},
		},
	})
	id, _, _ := e.pairFinding(t)
	err := assurance.NewStore(compose.InstallationDB(e.Pool)).Resolve(reader, id,
		assurance.Resolution{Outcome: assurance.OutcomeSeparate}, clocktest.Now(t))
	if err == nil {
		t.Fatal("a reader who may write only the later deal answered the pair separate")
	}
	if _, status, _ := e.pairFinding(t); status != "open" {
		t.Fatalf("status = %q after the refused answer, want open", status)
	}
	var answerable map[ids.UUID]bool
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var found assurance.Exception
		if err := tx.QueryRow(reader, `SELECT id, type, subject_kind, subject_id, observed FROM assurance_exception WHERE type = $1`,
			assurance.TypeDuplicateMotion).Scan(&found.ID, &found.Type, &found.SubjectKind, &found.SubjectID, &found.Observed); err != nil {
			return err
		}
		var aerr error
		answerable, aerr = assurance.AnswerableFindings(reader, tx, []assurance.Exception{found})
		return aerr
	}); err != nil {
		t.Fatal(err)
	}
	if answerable[id] {
		t.Fatal("the pair finding is offered as answerable to a reader who may not write its second deal")
	}
}

// A pass over the deal the finding is filed on closes it when that deal stops
// matching, whether its close date or its pipeline moved.
func TestAPassOverTheFiledDealClosesItsPairWhenItStopsMatching(t *testing.T) {
	for name, change := range map[string]func(*pairEnv){
		"close date": func(e *pairEnv) { e.setClose(t, e.later, 200) },
		"pipeline": func(e *pairEnv) {
			owner := integration.OwnerConn(t)
			other := integration.SeedIDRow(t, owner, `INSERT INTO pipeline (id, name) VALUES ($1, 'Other')`)
			stage := integration.SeedIDRow(t, owner, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Open', 0)`, other)
			execSeed(t, owner, `UPDATE deal SET pipeline_id = $2, stage_id = $3 WHERE id = $1`, e.later, other, stage)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupPair(t)
			e.run(t)
			change(e)
			e.rescan(t, e.later)
			if _, status, _ := e.pairFinding(t); status != assurance.ExceptionConditionCleared {
				t.Fatalf("status = %q, want condition_cleared", status)
			}
		})
	}
}

// A pair somebody deferred stays open through a pass over one deal, even when
// the pair no longer matches. The reminder fires over a clean record later.
func TestAPassOverOneDealKeepsADeferredPairOpen(t *testing.T) {
	e := setupPair(t)
	e.run(t)
	id, _, _ := e.pairFinding(t)
	remind := clocktest.Now(t).Add(48 * time.Hour)
	if err := assurance.NewStore(compose.InstallationDB(e.Pool)).Resolve(e.Admin(), id,
		assurance.Resolution{Outcome: assurance.OutcomeRemindLater, RemindAt: &remind}, clocktest.Now(t)); err != nil {
		t.Fatalf("deferring the pair: %v", err)
	}
	e.setClose(t, e.later, 200)
	e.rescan(t, e.later)
	if _, status, _ := e.pairFinding(t); status != "open" {
		t.Fatalf("status = %q after a pass over a pair that stopped matching, want the deferral kept open", status)
	}
}
