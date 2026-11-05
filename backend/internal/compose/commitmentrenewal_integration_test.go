// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLaterPassCannotReplaceANewerRenewalWithOlderEvidence(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	var messages []ids.UUID
	for i, age := range []int{45, 3, 1} {
		at := now.AddDate(0, 0, -age)
		body := []string{"I will send pricing.", "I will send pricing by Monday.", "I will send pricing by Friday instead."}[i]
		kind, source := "email", "manual"
		if i == 0 {
			// The original is imported history; a colleague's own late log would be proposed instead.
			kind, source = "note", "import"
		}
		message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: kind, Body: &body, OccurredAt: &at, ThreadKey: "renewal-order", Direction: new("outbound"), Source: source,
		})
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, ids.UUID(message.Id))
	}
	c := Commitment{
		Meaning: openCommitmentMeaning("", "I will send pricing."),
		AsOf:    now, ReadMessages: messages, Extractor: signalScanActor, SourceActivityID: messages[0],
		Summary: "Send pricing", Quote: "I will send pricing.", Seat: &e.Rep1, Confidence: 0.95, Locator: "pricing-renewal", Body: "I will send pricing.",
	}
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	dispatch := func() CommitmentOutcome {
		t.Helper()
		var outcome CommitmentOutcome
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			outcome, _, err = dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return outcome
	}
	if got := dispatch(); got != CommitmentOutcome(activities.CommitmentHistorical) {
		t.Fatalf("undated original = %s", got)
	}
	c.AdmissionSourceID, c.RenewedAt = &messages[2], new(now.AddDate(0, 0, -1))
	if got := dispatch(); got != CommitmentTaskWritten {
		t.Fatalf("undated renewal = %s", got)
	}
	if got := e.WsScalar(t, `SELECT renewed_by::text FROM activity_commitment_admission WHERE source_activity_id=$1`, messages[0]); got != messages[2].String() {
		t.Fatalf("renewal evidence = %s", got)
	}
	c.AdmissionSourceID, c.RenewedAt = &messages[1], new(now.AddDate(0, 0, -3))
	c.AsOf, c.DueDate = now.Add(time.Hour), "2026-10-19"
	if got := dispatch(); got != CommitmentRemembered {
		t.Fatalf("older evidence on a later pass = %s", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND due_at IS NULL`); got != 1 {
		t.Fatal("older evidence changed the renewed task")
	}
	privateMessage, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: new("I will send our private pricing tomorrow."), OccurredAt: &now,
		ThreadKey: "renewal-order", Direction: new("outbound"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	privateSource := ids.UUID(privateMessage.Id)
	c.AdmissionSourceID, c.RenewedAt, c.PrivateTo = &privateSource, &now, &e.Rep1
	c.ReadMessages = append(c.ReadMessages, privateSource)
	if got := dispatch(); got != CommitmentRemembered {
		t.Fatalf("private renewal changed a shared task: %s", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND due_at IS NULL AND audience='workspace'`); got != 1 {
		t.Fatal("private renewal changed the shared task's deadline or audience")
	}
}

func TestConcurrentOlderRenewalCannotOverwriteCommittedNewerWork(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	latest, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: new("I will send pricing next week."), OccurredAt: &now,
		ThreadKey: "renewal-privacy", Direction: new("outbound"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	latestID := ids.UUID(latest.Id)
	newer := Commitment{
		Meaning: openCommitmentMeaning("2026-10-23", "I will send pricing."),
		AsOf:    now.Add(time.Hour), ReadMessages: append(f.messages, latestID), Extractor: signalScanActor,
		SourceActivityID: f.messages[0], AdmissionSourceID: &latestID, RenewedAt: &now,
		Summary: "Send pricing next week", Quote: "I will send pricing.", Seat: &f.e.Rep1, PromisedTo: &f.contact,
		Confidence: 0.95, Locator: "private-renewal", Body: "Pricing promise", DueDate: "2026-10-23",
	}
	older := newer
	older.AsOf = now.Add(2 * time.Hour)
	older.AdmissionSourceID, older.RenewedAt, older.DueDate = &f.messages[1], new(now.Add(-24*time.Hour)), "2026-10-19"
	ctx, cancel := context.WithCancel(withTranscriptReader(principal.WithWorkspaceID(context.Background(), f.e.WS), "human:"+f.e.Rep1.String(), ids.NewV7()))
	defer cancel()
	dispatcher := NewCommitmentDispatcher(f.e.Pool, approvals.NewService(f.e.DB()))
	held := make(chan uint32, 1)
	release := make(chan struct{})
	winner := make(chan error, 1)
	go func() {
		winner <- database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
			outcome, _, err := dispatcher.DispatchTx(ctx, tx, newer, ids.NewV7())
			if err != nil {
				return err
			}
			if outcome != CommitmentRetained {
				return fmt.Errorf("newer renewal = %s", outcome)
			}
			held <- tx.Conn().PgConn().PID()
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var pid uint32
	select {
	case pid = <-held:
	case err := <-winner:
		t.Fatalf("newer writer failed before locking: %v", err)
	}
	done := make(chan struct{})
	loser := make(chan error, 1)
	go func() {
		defer close(done)
		loser <- database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
			outcome, _, err := dispatcher.DispatchTx(ctx, tx, older, ids.NewV7())
			if err == nil && outcome != CommitmentRemembered {
				return fmt.Errorf("stale renewal = %s", outcome)
			}
			return err
		})
	}()
	observer := integration.OwnerConn(t)
	testdb.WaitForContention(t, done, "older renewal bypassed the source lock", "older renewal never reached the source lock", func(probe context.Context) (bool, error) {
		if _, err := observer.Exec(probe, `SELECT pg_stat_clear_snapshot()`); err != nil {
			return false, err
		}
		var blocked bool
		err := observer.QueryRow(probe, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked)
		return blocked, err
	})
	close(release)
	if err := <-winner; err != nil {
		t.Fatal(err)
	}
	if err := <-loser; err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`SELECT to_char(due_at,'YYYY-MM-DD') FROM activity WHERE id=$1`,
		`SELECT to_char(due_at,'YYYY-MM-DD') FROM conversation_claim WHERE source_activity_id=(SELECT source_activity_id FROM activity WHERE id=$1)`,
	} {
		if got := f.e.WsScalar(t, query, f.work); got != "2026-10-23" {
			t.Fatalf("concurrent deadline = %s", got)
		}
	}
	if got := f.e.WsScalar(t, `SELECT renewed_by::text FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0]); got != latestID.String() {
		t.Fatalf("stale writer replaced provenance with %s", got)
	}
}

func TestErasingOldRenewalWaitsForTheTaskAndRechecksItsEvidence(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	subject := f.e.SeedContact(t, "Old renewal subject", &f.e.Rep1)
	integration.LinkActivity(t, integration.OwnerConn(t), f.messages[1], "contact", subject)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	later, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: new("I will send the new pricing next week."), OccurredAt: &now,
		Source: "manual", Direction: new("outbound"), ThreadKey: "renewal-privacy",
	})
	if err != nil {
		t.Fatal(err)
	}
	renewed := ids.UUID(later.Id)
	c := Commitment{
		Meaning: openCommitmentMeaning("2026-10-23", "I will send pricing."),
		AsOf:    now.Add(time.Hour), ReadMessages: append(f.messages, renewed), Extractor: signalScanActor,
		SourceActivityID: f.messages[0], AdmissionSourceID: &renewed, RenewedAt: &now,
		Summary: "Send new pricing", Quote: "I will send pricing.", Seat: &f.e.Rep1, PromisedTo: &f.contact,
		Confidence: 0.95, Locator: "private-renewal", DueDate: "2026-10-23",
	}
	ctx, cancel := context.WithCancel(withTranscriptReader(principal.WithWorkspaceID(context.Background(), f.e.WS), "human:"+f.e.Rep1.String(), ids.NewV7()))
	defer cancel()
	eraseCtx, stopErase := context.WithCancel(f.e.Admin())
	defer stopErase()
	done, erased := make(chan struct{}), make(chan error, 1)
	observer := integration.OwnerConn(t)
	dispatcher := NewCommitmentDispatcher(f.e.Pool, approvals.NewService(f.e.DB()))
	err = database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		if _, err := storekit.LockRow(ctx, tx, "activity", f.work, storekit.IncludeArchived); err != nil {
			return err
		}
		pid := tx.Conn().PgConn().PID()
		go func() {
			defer close(done)
			erased <- privacy.NewEraser(f.e.DB()).WithRawCapturePurger(RawCapturePurgerFor(f.e.DB())).EraseContact(eraseCtx, subject, "Subject requested erasure")
		}()
		testdb.WaitForContention(t, done, "erasure did not wait for task renewal", "erasure never reached the held task", func(probe context.Context) (bool, error) {
			if _, err := observer.Exec(probe, `SELECT pg_stat_clear_snapshot()`); err != nil {
				return false, err
			}
			var blocked bool
			err := observer.QueryRow(probe, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked)
			return blocked, err
		})
		_, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-erased; err != nil {
		t.Fatal(err)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body IS NULL`, f.messages[1]); got != 1 {
		t.Fatal("old renewal was not erased")
	}
	if got := f.e.WsScalar(t, `SELECT subject FROM activity WHERE id=$1 AND archived_at IS NULL`, f.work); got != "Send new pricing" {
		t.Fatalf("erasure destroyed newer work: %s", got)
	}
	if got := f.e.WsScalar(t, `SELECT renewed_by::text FROM activity_commitment_admission WHERE source_activity_id=$1`, f.messages[0]); got != renewed.String() {
		t.Fatalf("erasure removed newer provenance: %s", got)
	}
}
