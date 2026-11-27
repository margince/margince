// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordsources

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// olderSignal plants a signal the way a row was written before the source
// check existed: the producer's name, or a client's free text, in source.
func olderSignal(t *testing.T, owner *pgx.Conn, kind, summary, source string) ids.UUID {
	t.Helper()
	return integration.SeedBeforeSourceCheck(t, owner, "signal",
		`INSERT INTO signal (id, kind, resolution_state, summary, source, captured_by)
		 VALUES ($1, $2, 'unresolved', $3, $4, 'system:x')`, kind, summary, source)
}

// signalOrigin reads a signal's channel and its producer, empty when it has none.
func signalOrigin(t *testing.T, e *integration.Env, id ids.UUID) [2]string {
	t.Helper()
	return [2]string{
		e.WsScalar(t, `SELECT source FROM signal WHERE id = $1`, id),
		e.WsScalar(t, `SELECT COALESCE(source_system, '') FROM signal WHERE id = $1`, id),
	}
}

// A producer's name moves to source_system and the row lands on automation. A
// client's free text names no producer, and lands on api with none.
func TestTheTaskMovesAnOlderSignalsProducerIntoSourceSystem(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	want := map[ids.UUID][2]string{
		olderSignal(t, owner, "ghosted_thread", "No reply", "signal-scan"):         {"automation", "signal-scan"},
		olderSignal(t, owner, "funding", "Raised a round", "site-newsroom"):        {"automation", "site-newsroom"},
		olderSignal(t, owner, "technical_change", "New stack", "technical-lookup"): {"automation", "technical-lookup"},
		olderSignal(t, owner, "ext:billing:paid_late", "Paid late", "billing"):     {"automation", "billing"},
		olderSignal(t, owner, "risk", "Budget freeze", "analyst-note"):             {"api", ""},
		olderSignal(t, owner, "risk", "Their champion left", "ui"):                 {"manual", ""},
		olderSignal(t, owner, "risk", "Filed by a member", "manual"):               {"manual", ""},
		olderSignal(t, owner, "ext:billing:paid_late", "Filed on the list", "api"): {"automation", "api"},
	}
	scan := ids.UUID{}
	for id, origin := range want {
		if origin[1] == "signal-scan" {
			scan = id
		}
	}

	ctx := systemCtx(e)
	task := SignalDataTask()
	if n, capped, err := task.Steps[0].Estimate(ctx, e.Pool, 3); err != nil || n != 3 || !capped {
		t.Fatalf("the signal estimate under a cap of 3 = %d, capped %v (%v), want 3 and capped", n, capped, err)
	}
	e.RunDataTask(ctx, t, task)

	for id, origin := range want {
		if got := signalOrigin(t, e, id); got != origin {
			t.Errorf("signal %s reads source and source_system %v, want %v", id, got, origin)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'signal' AND entity_id = $1
		AND action = 'update' AND before->>'source' = 'signal-scan' AND after->>'source' = 'automation'
		AND evidence ? 'data_task_run'`, scan); n != 1 {
		t.Errorf("%d audit row(s) keep the scan signal's older source, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'signal' AND action = 'update'`); n != 7 {
		t.Errorf("%d signal(s) were audited as moved, want all but the member's", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'type' = 'signal'`); n != 0 {
		t.Errorf("the corrected signals sent %d event(s), want none", n)
	}
	if n, _, err := task.Steps[0].Estimate(ctx, e.Pool, 10); err != nil || n != 0 {
		t.Errorf("the step still estimates %d signal(s) (%v), want none", n, err)
	}
}

// The scan's readers select its rows by producer. A row the task has not
// reached still names the scan in source, and a reader must find it there.
func TestAScanReaderFindsAnOlderSignalBeforeTheTaskHasRun(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	scanned := olderSignal(t, owner, "ghosted_thread", "No reply for 15 days", "signal-scan")
	filed := olderSignal(t, owner, "ghosted_thread", "No reply for 15 days", "analyst-note")

	ctx := systemCtx(e)
	reword := func(id ids.UUID) (reworded bool) {
		t.Helper()
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) (err error) {
			reworded, err = signals.RewordScanSummaryTx(ctx, tx, signals.ScanSummaryReword{
				Signal: id, From: "No reply for 15 days", To: "No reply for a while",
			})
			return err
		}); err != nil {
			t.Fatalf("rewording signal %s: %v", id, err)
		}
		return reworded
	}
	if !reword(scanned) {
		t.Error("an older scan signal was not reworded: the reader no longer finds it")
	}
	if reword(filed) {
		t.Error("a signal a client filed was reworded as the scan's")
	}
	if got, want := signalOrigin(t, e, scanned), [2]string{"automation", "signal-scan"}; got != want {
		t.Errorf("the reworded scan signal reads %v, want %v: the edit moves it onto its channel", got, want)
	}
	if got := e.WsScalar(t, `SELECT summary FROM signal WHERE id = $1`, scanned); got != "No reply for a while" {
		t.Errorf("the moved scan signal's summary = %q, want the reworded one", got)
	}
}

// An unvalidated check still judges an older row when it is next written. A
// triage before the task has run goes through, and moves the producer too.
func TestATriageBeforeTheTaskMovesAnOlderSignalOntoItsChannel(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	unit := olderSignal(t, owner, "ext:billing:paid_late", "Paid late", "billing")
	client := olderSignal(t, owner, "risk", "Budget freeze", "analyst-note")
	for _, id := range []ids.UUID{unit, client} {
		if _, err := owner.Exec(context.Background(),
			`UPDATE signal SET status = 'acknowledged' WHERE id = $1`, id); err != nil {
			t.Fatalf("triaging an older signal: %v", err)
		}
	}
	if got, want := signalOrigin(t, e, unit), [2]string{"automation", "billing"}; got != want {
		t.Errorf("the extension's signal reads %v after the triage, want %v", got, want)
	}
	if got, want := signalOrigin(t, e, client), [2]string{"api", ""}; got != want {
		t.Errorf("the client's signal reads %v after the triage, want %v", got, want)
	}
}

// A new scan finding is written on its channel with its producer beside it.
func TestANewScanFindingNamesItsChannelAndItsProducer(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Quiet Co", nil)
	ctx := systemCtx(e)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind: "ghosted_thread", CompanyID: company, Summary: "No reply", Severity: "warn", Fingerprint: "quiet-co",
		}, time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatalf("recording a scan finding: %v", err)
	}
	found := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM signal WHERE fingerprint = 'quiet-co'`))
	if got, want := signalOrigin(t, e, found), [2]string{"automation", "signal-scan"}; got != want {
		t.Errorf("a new scan finding reads %v, want %v", got, want)
	}
}
