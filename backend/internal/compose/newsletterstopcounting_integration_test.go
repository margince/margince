// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (f *relationshipFixture) runNewsletterTask(t *testing.T) {
	t.Helper()
	task := newslettersDataTask(f.e.Pool)
	for i := range task.Steps {
		task.Steps[i].Limit, task.Steps[i].MinLimit = 1, 1
	}
	now := clocktest.Now(t)
	engine, err := datatask.New(f.e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")
	if err == nil {
		err = engine.Seed(ctx)
	}
	for i := 0; err == nil && i < 40; i++ {
		_, err = engine.Execute(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestANewsletterMovesNoClockAndTheDataTaskTakesOneBack(t *testing.T) {
	f := newRelationshipFixture(t)
	f.humanExchange(t)
	before := workspaceValue[*time.Time](t, f.e, `SELECT last_activity_at FROM contact WHERE id = $1`, f.contact)
	newsletter := f.mail(t, "client-newsletter", f.contactAddr, f.seatAddr, "", 4, newsletterBody("Our autumn newsletter.")...)
	requireBulkStamp(t, f, newsletter)
	if got := workspaceValue[*time.Time](t, f.e, `SELECT last_activity_at FROM contact WHERE id = $1`, f.contact); got == nil || before == nil || !got.Equal(*before) {
		t.Fatalf("a newsletter moved the clock to %v, want %v", got, before)
	}
	f.e.WsExec(t, `UPDATE contact SET last_activity_at = $1 WHERE id = $2`, instantOf(t, f.e, newsletter), f.contact)
	f.runNewsletterTask(t)
	if got := workspaceValue[*time.Time](t, f.e, `SELECT last_activity_at FROM contact WHERE id = $1`, f.contact); got == nil || !got.Equal(*before) {
		t.Errorf("the repaired clock is %v, want the human answer's %v", got, before)
	}
}

func (f *relationshipFixture) ghosted(t *testing.T, activity ids.UUID) int {
	t.Helper()
	ctx := principal.SystemActing(f.e.Admin(), signalScanActor)
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		_, err := WriteGhostedSignals(ctx, tx, f.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return workspaceValue[int](t, f.e, `SELECT count(*)::int FROM signal WHERE kind = $1 AND resolved_company_id = $2
		 AND archived_at IS NULL AND evidence @> jsonb_build_array(jsonb_build_object('source_type', 'activity', 'source_id', $3::text))`, kindGhostedThread, f.company, activity.String())
}

func TestTheDataTaskArchivesOnlyAGhostedThreadOurNewsletterRaised(t *testing.T) {
	for left, body := range [][]string{newsletterBody("News from us."), nil} {
		f := newRelationshipFixture(t)
		f.e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, f.company)
		f.mail(t, "pricing-question", f.contactAddr, f.seatAddr, "", 30)
		ours := f.mail(t, "our-mail", f.seatAddr, f.contactAddr, "", ghostedThresholdDays+2, body...)
		if body != nil {
			requireBulkStamp(t, f, ours)
			if got := f.ghosted(t, ours); got != 0 {
				t.Fatalf("our newsletter raised %d ghosted-thread signals, want none", got)
			}
			f.e.WsExec(t, `UPDATE activity SET bulk_mail_attested = false WHERE id = $1`, ours)
		}
		if got := f.ghosted(t, ours); got != 1 {
			t.Fatalf("the scan raised %d ghosted-thread signals, want 1", got)
		}
		f.e.WsExec(t, `UPDATE activity SET bulk_mail_attested = $2 WHERE id = $1`, ours, body != nil)
		f.runNewsletterTask(t)
		if got := f.ghosted(t, ours); got != left {
			t.Errorf("%d ghosted-thread signals left, want %d", got, left)
		}
	}
}
