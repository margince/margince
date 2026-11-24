// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package purgerun_test

// The run mechanism against a real database: a run that stopped before its
// receipt gets one, and runs of one rule never merge.

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/purgerun"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const address = "gegner@example.test"

func ctxFor(e *integration.Env, user ids.UUID) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Update: true, Delete: true},
				"contact":  {Read: true, Create: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func retentionFor(e *integration.Env) *privacy.RetentionService {
	return compose.NewRetentionServiceFor(compose.InstallationDB(e.Pool), nil, slog.Default())
}

func purgerFor(e *integration.Env) *compose.CapturePurger {
	return compose.NewCapturePurger(e.Pool, retentionFor(e))
}

func seedMail(t *testing.T, e *integration.Env, from, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, audience)
			VALUES ($1, 'email', $2, 'the message body', 'inbound', 'gmail', $3,
			        'gmail:'||$3, 'connector:gmail', $4, 'participants')`,
			id, subject, "pg-"+id.String(), from); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import)
			VALUES ($1, $2, 'classified')`, id, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding mail: %v", err)
	}
	return id
}

func seedRule(t *testing.T, e *integration.Env, value string) ids.UUID {
	t.Helper()
	rule, err := capture.NewExclusionStore(compose.InstallationDB(e.Pool)).
		Add(ctxFor(e, e.Rep1), capture.ExclusionScopeUser, capture.ExclusionKindAddress, value, ids.Nil)
	if err != nil {
		t.Fatalf("adding the rule: %v", err)
	}
	return rule.ID
}

// destroyAsRun destroys the mails as the cascade does, under the run's context.
// It writes no receipt, as a process that died before the receipt would.
func destroyAsRun(ctx context.Context, e *integration.Env, mails []ids.UUID, reason privacy.PurgeReason) error {
	_, err := retentionFor(e).PurgeActivities(ctx, mails, reason)
	return err
}

func stoppedPurge(t *testing.T, e *integration.Env, rule ids.UUID, mails ...ids.UUID) {
	t.Helper()
	err := purgerun.Hold(ctxFor(e, e.Rep1), e.Pool, purgerun.Rule(rule), func(ctx context.Context, _ ids.UUID) error {
		return destroyAsRun(ctx, e, mails, privacy.PurgeOwnerRule)
	})
	if err != nil {
		t.Fatalf("stopped purge: %v", err)
	}
}

func receipts(t *testing.T, e *integration.Env, entity string, subject ids.UUID) []int {
	t.Helper()
	var out []int
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT (evidence->>'destroyed')::int FROM audit_log
			WHERE entity_type = $1 AND entity_id = $2 AND evidence ? 'destroyed' ORDER BY id`, entity, subject)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[int])
		return err
	}); err != nil {
		t.Fatalf("reading receipts: %v", err)
	}
	return out
}

func sweep(t *testing.T, e *integration.Env) purgerun.SweepResult {
	t.Helper()
	result, err := purgerun.Sweep(ctxFor(e, e.Rep1), e.Pool)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	return result
}

// Rows without a receipt get one receipt, written from the rows, and another
// rule's certified purge is left as it was.
func TestTheSweepWritesTheMissingReceiptOnce(t *testing.T) {
	e := integration.Setup(t)
	first := seedMail(t, e, address, "Eins")
	rule := seedRule(t, e, address)
	seedMail(t, e, "andere@example.test", "Drei")
	other := seedRule(t, e, "andere@example.test")
	if _, err := purgerFor(e).Purge(ctxFor(e, e.Rep1), other, false); err != nil {
		t.Fatalf("purging the other rule: %v", err)
	}

	stoppedPurge(t, e, rule, first)
	if got := receipts(t, e, purgerun.EntityRule, rule); len(got) != 0 {
		t.Fatalf("receipts before the sweep = %v, want none", got)
	}
	if result := sweep(t, e); result.Written != 1 || result.Unattributed != 0 {
		t.Fatalf("first sweep = %+v, want one receipt written", result)
	}
	if result := sweep(t, e); result.Written != 0 {
		t.Fatalf("second sweep = %+v, want nothing more", result)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND batch_id IS NOT NULL
		  AND (evidence->>'destroyed')::int = 1 AND (evidence->>'recovered')::boolean
		  AND actor_type = 'system'`, rule); got != 1 {
		t.Fatalf("recovered receipts of the rule = %d, want 1", got)
	}
	if got := receipts(t, e, purgerun.EntityRule, other); !slices.Equal(got, []int{1}) {
		t.Fatalf("the other rule's receipts = %v, want its own one untouched", got)
	}
}

// Two purges of one rule run one after the other, and each certifies only what
// it destroyed.
func TestTwoConcurrentPurgesOfOneRuleGetTheirOwnReceipts(t *testing.T) {
	e := integration.Setup(t)
	seedMail(t, e, address, "Eins")
	seedMail(t, e, address, "Zwei")
	rule := seedRule(t, e, address)

	outcomes := make([]compose.PurgeOutcome, 2)
	var wg sync.WaitGroup
	for i := range outcomes {
		wg.Go(func() {
			var err error
			if outcomes[i], err = purgerFor(e).Purge(ctxFor(e, e.Rep1), rule, false); err != nil {
				t.Errorf("purge %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	got := receipts(t, e, purgerun.EntityRule, rule)
	want := []int{outcomes[0].Destroyed, outcomes[1].Destroyed}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("receipts = %v, outcomes = %v, want one receipt per purge stating that purge's own count", got, want)
	}
}

// A row that commits after another run's receipt belongs to its own run's
// receipt, and to no other.
func TestARowCommittedAfterAnotherRunsReceiptIsCountedByItsOwnRun(t *testing.T) {
	e := integration.Setup(t)
	a, b := seedMail(t, e, address, "Eins"), seedMail(t, e, address, "Zwei")
	rule := seedRule(t, e, address)
	runA, runB := purgerun.NewRunID(rule), purgerun.NewRunID(rule)
	for run, mail := range map[ids.UUID]ids.UUID{runA: a, runB: b} {
		if err := destroyAsRun(storekit.WithBatch(ctxFor(e, e.Rep1), run), e, []ids.UUID{mail}, privacy.PurgeOwnerRule); err != nil {
			t.Fatalf("destroying: %v", err)
		}
	}
	for _, run := range []ids.UUID{runB, runA} {
		tally, err := purgerun.WriteReceipt(ctxFor(e, e.Rep1), e.Pool, purgerun.Rule(rule), run,
			func(t purgerun.Tally) map[string]any { return t.Evidence() })
		if err != nil || tally.Destroyed != 1 {
			t.Fatalf("receipt of run %s = (%+v, %v), want its own 1 message", run, tally, err)
		}
	}
}

// Removing a rule after a purge that stopped leaves the receipt behind.
func TestRemovingARuleAfterAStoppedPurgeLeavesItsReceipt(t *testing.T) {
	e := integration.Setup(t)
	mail := seedMail(t, e, address, "Eins")
	rule := seedRule(t, e, address)
	stoppedPurge(t, e, rule, mail)

	store := capture.NewExclusionStore(compose.InstallationDB(e.Pool)).WithRemovalRepair(purgerun.RepairRule)
	if err := store.Remove(ctxFor(e, e.Rep1), rule); err != nil {
		t.Fatalf("removing the rule: %v", err)
	}
	if got := receipts(t, e, purgerun.EntityRule, rule); !slices.Equal(got, []int{1}) {
		t.Fatalf("receipts after the removal = %v, want the stopped run's", got)
	}
	if result := sweep(t, e); result.Written != 0 || result.Unattributed != 0 {
		t.Fatalf("sweep after the removal = %+v, want nothing left to do", result)
	}
}

// A rule purged again after new mail arrived gets a second receipt of its own.
func TestASecondPurgeOfOneRuleGetsItsOwnReceipt(t *testing.T) {
	e := integration.Setup(t)
	seedMail(t, e, address, "Eins")
	rule := seedRule(t, e, address)
	if _, err := purgerFor(e).Purge(ctxFor(e, e.Rep1), rule, false); err != nil {
		t.Fatalf("first purge: %v", err)
	}
	seedMail(t, e, address, "Zwei")
	second, err := purgerFor(e).Purge(ctxFor(e, e.Rep1), rule, false)
	if err != nil {
		t.Fatalf("second purge: %v", err)
	}
	if got := receipts(t, e, purgerun.EntityRule, rule); !slices.Equal(got, []int{1, second.Destroyed}) || second.Destroyed < 1 {
		t.Fatalf("receipts = %v, second purge = %d, want one receipt per purge with its own count", got, second.Destroyed)
	}
}

// The sweep leaves a run alone while it holds its lock, and certifies it once
// the run is over without a receipt.
func TestTheSweepLeavesARunInFlightAlone(t *testing.T) {
	e := integration.Setup(t)
	mail := seedMail(t, e, address, "Eins")
	rule := seedRule(t, e, address)
	err := purgerun.Hold(ctxFor(e, e.Rep1), e.Pool, purgerun.Rule(rule), func(ctx context.Context, _ ids.UUID) error {
		if err := destroyAsRun(ctx, e, []ids.UUID{mail}, privacy.PurgeOwnerRule); err != nil {
			return err
		}
		if result := sweep(t, e); result.Written != 0 {
			t.Errorf("sweep during the run = %+v, want it to leave the run alone", result)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result := sweep(t, e); result.Written != 1 {
		t.Fatalf("sweep after the run = %+v, want its receipt", result)
	}
}

// A seat's personal-mail pass that stopped gets its receipt the same way.
func TestTheSweepCertifiesAStoppedPersonalPass(t *testing.T) {
	e := integration.Setup(t)
	mail := seedMail(t, e, address, "Eins")
	err := purgerun.Hold(ctxFor(e, e.Rep1), e.Pool, purgerun.Seat(e.Rep1), func(ctx context.Context, _ ids.UUID) error {
		return destroyAsRun(ctx, e, []ids.UUID{mail}, privacy.PurgePersonalVerdict)
	})
	if err != nil {
		t.Fatalf("personal pass: %v", err)
	}
	if result := sweep(t, e); result.Written != 1 {
		t.Fatalf("sweep = %+v, want the seat's receipt", result)
	}
	if got := receipts(t, e, purgerun.EntityPersonalSweep, e.Rep1); !slices.Equal(got, []int{1}) {
		t.Fatalf("personal receipts = %v, want one for the 1 destroyed message", got)
	}
}

// A redaction can strip what a release row carried, so a release is counted by
// its run and not by what the row says.
func TestAReleaseIsStillCountedAfterItsMessageIsErased(t *testing.T) {
	e := integration.Setup(t)
	shared := seedMail(t, e, address, "Geteilt")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO capture_import (activity_id, user_id, posture_at_import)
			VALUES ($1, $2, 'classified')`, shared, e.Rep2)
		return err
	}); err != nil {
		t.Fatalf("adding a second importer: %v", err)
	}
	rule := seedRule(t, e, address)
	if _, err := purgerFor(e).Purge(ctxFor(e, e.Rep1), rule, false); err != nil {
		t.Fatalf("purging: %v", err)
	}
	run := ids.MustParse(e.WsScalar(t, `SELECT batch_id::text FROM audit_log
		WHERE entity_type = 'capture_exclusion' AND entity_id = $1 AND evidence ? 'destroyed'`, rule))
	if err := destroyAsRun(ctxFor(e, e.Rep1), e, []ids.UUID{shared}, privacy.PurgeOwnerRule); err != nil {
		t.Fatalf("erasing the released message: %v", err)
	}
	if left := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'archive' AND after ? 'released_by'`, shared); left != 0 {
		t.Fatalf("the release row still shows its image (%d), so this test proves nothing", left)
	}
	var tally purgerun.Tally
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) (err error) {
		tally, err = purgerun.TallyTx(e.Admin(), tx, run)
		return err
	}); err != nil || tally.Released != 1 {
		t.Fatalf("tally = (%+v, %v), want the release counted", tally, err)
	}
}
