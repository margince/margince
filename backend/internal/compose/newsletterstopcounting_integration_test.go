// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A newsletter moves no last-activity clock and raises no ghosted thread. The
// data task takes back the clocks and signals one left before that held.
// The mail lands through capture, which stamps the List-Unsubscribe header.

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

// clocksOf reads the stored last-activity clock of the contact and its company.
func (f *relationshipFixture) clocksOf(t *testing.T) (contact, company *time.Time) {
	t.Helper()
	contact = workspaceValue[*time.Time](t, f.e, `SELECT last_activity_at FROM contact WHERE id = $1`, f.contact)
	company = workspaceValue[*time.Time](t, f.e, `SELECT last_activity_at FROM company WHERE id = $1`, f.company)
	return contact, company
}

func (f *relationshipFixture) assertClocksAt(t *testing.T, want *time.Time, why string) {
	t.Helper()
	contact, company := f.clocksOf(t)
	for name, at := range map[string]*time.Time{"contact": contact, "company": company} {
		if at == nil || want == nil || !at.Equal(*want) {
			t.Errorf("%s last_activity_at = %v, want %v: %s", name, at, want, why)
		}
	}
}

// runNewsletterTask runs the data task to its end, one record per batch.
func (f *relationshipFixture) runNewsletterTask(t *testing.T) {
	t.Helper()
	task := newslettersDataTask(f.e.Pool)
	for i := range task.Steps {
		task.Steps[i].Limit, task.Steps[i].MinLimit = 1, 1
	}
	now := clocktest.Now(t)
	engine, err := datatask.New(f.e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 40 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestANewsletterMovesNoLastActivityClock(t *testing.T) {
	f := newRelationshipFixture(t)
	f.humanExchange(t)
	contact, _ := f.clocksOf(t)
	newsletter := f.mail(t, "client-newsletter", f.contactAddr, f.seatAddr, "", 4, newsletterBody("Our autumn newsletter.")...)
	requireBulkStamp(t, f, newsletter)
	f.assertClocksAt(t, contact, "a newsletter is no contact")
}

func TestTheDataTaskTakesANewsletterOutOfAStoredClock(t *testing.T) {
	f := newRelationshipFixture(t)
	f.humanExchange(t)
	before, _ := f.clocksOf(t)
	newsletter := f.mail(t, "client-newsletter", f.contactAddr, f.seatAddr, "", 4, newsletterBody("Our autumn newsletter.")...)
	requireBulkStamp(t, f, newsletter)
	sent := instantOf(t, f.e, newsletter)
	// The value the clocks held while a newsletter still counted.
	f.e.WsExec(t, `UPDATE contact SET last_activity_at = $1 WHERE id = $2`, sent, f.contact)
	f.e.WsExec(t, `UPDATE company SET last_activity_at = $1 WHERE id = $2`, sent, f.company)

	f.runNewsletterTask(t)
	f.assertClocksAt(t, before, "the repaired clock falls back to the human answer")
}

// ghostedCiting reads the live ghosted-thread signals on the company that cite
// the activity.
func (f *relationshipFixture) ghostedCiting(t *testing.T, activity ids.UUID) int {
	t.Helper()
	return workspaceValue[int](t, f.e, `SELECT count(*)::int FROM signal
		 WHERE kind = $1 AND resolved_company_id = $2 AND archived_at IS NULL
		   AND evidence @> jsonb_build_array(jsonb_build_object('source_type', 'activity', 'source_id', $3::text))`,
		kindGhostedThread, f.company, activity.String())
}

func (f *relationshipFixture) scanGhosted(t *testing.T) {
	t.Helper()
	ctx := principal.SystemActing(f.e.Admin(), signalScanActor)
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		_, err := WriteGhostedSignals(ctx, tx, f.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// ourNewsletterUnanswered is an engaged account whose newest mail is our
// newsletter, sent past the ghosted threshold after their last word.
func ourNewsletterUnanswered(t *testing.T) (*relationshipFixture, ids.UUID) {
	t.Helper()
	f := newRelationshipFixture(t)
	f.e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, f.company)
	f.mail(t, "pricing-question", f.contactAddr, f.seatAddr, "", 30)
	ours := f.mail(t, "our-newsletter", f.seatAddr, f.contactAddr, "", ghostedThresholdDays+2, newsletterBody("News from us.")...)
	requireBulkStamp(t, f, ours)
	return f, ours
}

func TestOurNewsletterRaisesNoGhostedThread(t *testing.T) {
	f, ours := ourNewsletterUnanswered(t)
	f.scanGhosted(t)
	if got := f.ghostedCiting(t, ours); got != 0 {
		t.Errorf("our newsletter raised %d ghosted-thread signals, want none", got)
	}
}

func TestTheDataTaskArchivesAGhostedThreadOurNewsletterRaised(t *testing.T) {
	f, ours := ourNewsletterUnanswered(t)
	// The signal the scan raised while a newsletter still counted.
	f.e.WsExec(t, `UPDATE activity SET bulk_mail_attested = false WHERE id = $1`, ours)
	f.scanGhosted(t)
	if got := f.ghostedCiting(t, ours); got != 1 {
		t.Fatalf("an unstamped newsletter raised %d ghosted-thread signals, want 1, so the repair below proves nothing", got)
	}
	f.e.WsExec(t, `UPDATE activity SET bulk_mail_attested = true WHERE id = $1`, ours)

	f.runNewsletterTask(t)
	if got := f.ghostedCiting(t, ours); got != 0 {
		t.Errorf("%d ghosted-thread signals still cite our newsletter, want none", got)
	}
}

// A ghosted thread on a mail a human wrote stays.
func TestTheDataTaskKeepsAGhostedThreadOnHumanMail(t *testing.T) {
	f := newRelationshipFixture(t)
	f.e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, f.company)
	f.mail(t, "pricing-question", f.contactAddr, f.seatAddr, "", 30)
	ours := f.mail(t, "pricing-answer", f.seatAddr, f.contactAddr, "", ghostedThresholdDays+2)
	f.scanGhosted(t)
	if got := f.ghostedCiting(t, ours); got != 1 {
		t.Fatalf("our unanswered mail raised %d ghosted-thread signals, want 1", got)
	}
	f.runNewsletterTask(t)
	if got := f.ghostedCiting(t, ours); got != 1 {
		t.Errorf("the data task archived a ghosted thread on human mail: %d left, want 1", got)
	}
}
