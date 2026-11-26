// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A newsletter is no touch. The repair reopens the quiet reminders the engine
// closed on one before that held, and leaves every other closed reminder alone.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// seedNewsletter files a received mail that carried a List-Unsubscribe header,
// linked to the deal.
func (e *recencyEnv) seedNewsletter(t *testing.T, deal ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	mail := ids.NewV7()
	e.exec(t, `INSERT INTO activity
		  (id, kind, direction, subject, occurred_at, audience, source, captured_by, origin, bulk_mail_attested)
		VALUES ($1, 'email', 'inbound', 'Our autumn newsletter', $2, 'workspace', 'gmail', 'connector:gmail', 'human', true)`,
		mail, at)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), mail, deal)
	return mail
}

// closeAs ticks the reminder through the store's update, the write the engine's
// completion and a member's tick both take.
func (e *recencyEnv) closeAs(ctx context.Context, t *testing.T, reminder ids.UUID) {
	t.Helper()
	done := true
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	if _, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](reminder), UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatalf("closing the reminder: %v", err)
	}
}

// asEngine is the automation engine's posture when it completes a reminder.
func (e *recencyEnv) asEngine() context.Context {
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), systemCapturedBy)
}

// repairNewsletterReminders runs the repair over every reminder in one page.
func (e *recencyEnv) repairNewsletterReminders(t *testing.T) int {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), "system:data-task-test")
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	var reopened int
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) (err error) {
		_, reopened, err = store.ReopenNewsletterClosedRemindersTx(ctx, tx, ids.Nil, 1000)
		return err
	}); err != nil {
		t.Fatalf("repairing the reminders: %v", err)
	}
	return reopened
}

func TestTheRepairReopensAReminderTheEngineClosedOnANewsletter(t *testing.T) {
	e := setupRecency(t)
	deal := e.seedQuietDeal(t, pinnedQuiet)
	reminder := e.seedQuietReminder(t, deal, pinnedQuiet)
	e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, 89))
	e.closeAs(e.asEngine(), t, reminder)

	if got := e.repairNewsletterReminders(t); got != 1 {
		t.Fatalf("the repair reopened %d reminders, want 1", got)
	}
	if e.reminderIsDone(t, reminder) {
		t.Fatal("the reminder a newsletter closed is still closed")
	}
	if got := e.repairNewsletterReminders(t); got != 0 {
		t.Errorf("a second run reopened %d reminders, want none", got)
	}
}

func TestTheRepairLeavesEveryOtherClosedReminderAlone(t *testing.T) {
	for _, test := range []struct {
		name  string
		close func(e *recencyEnv, t *testing.T, deal, reminder ids.UUID)
	}{
		{"a member ticked it", func(e *recencyEnv, t *testing.T, deal, reminder ids.UUID) {
			e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, 89))
			e.closeAs(e.asResolver(), t, reminder)
		}},
		{"another system writer closed it", func(e *recencyEnv, t *testing.T, deal, reminder ids.UUID) {
			e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, 89))
			e.closeAs(principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), "system:review-close"), t, reminder)
		}},
		{"a member worked on it after the engine closed it", func(e *recencyEnv, t *testing.T, deal, reminder ids.UUID) {
			e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, 89))
			e.closeAs(e.asEngine(), t, reminder)
			subject := "Followed up by phone"
			store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
			if _, err := store.UpdateActivity(e.asResolver(), ids.From[ids.ActivityKind](reminder), UpdateActivityInput{Subject: &subject}); err != nil {
				t.Fatal(err)
			}
		}},
		{"a call reached the record after the anchor", func(e *recencyEnv, t *testing.T, deal, reminder ids.UUID) {
			e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, 88))
			e.closeAs(e.asEngine(), t, reminder)
			e.seedCall(t, deal, pinnedQuiet.AddDate(0, 0, 89))
		}},
		{"no newsletter reached the record", func(e *recencyEnv, t *testing.T, _, reminder ids.UUID) {
			e.closeAs(e.asEngine(), t, reminder)
		}},
		{"the newsletter came before the anchor", func(e *recencyEnv, t *testing.T, deal, reminder ids.UUID) {
			e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, -1))
			e.closeAs(e.asEngine(), t, reminder)
		}},
		{"the newsletter was captured after the close", func(e *recencyEnv, t *testing.T, deal, reminder ids.UUID) {
			e.closeAs(e.asEngine(), t, reminder)
			e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, 89))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := setupRecency(t)
			deal := e.seedQuietDeal(t, pinnedQuiet)
			reminder := e.seedQuietReminder(t, deal, pinnedQuiet)
			test.close(e, t, deal, reminder)
			if got := e.repairNewsletterReminders(t); got != 0 {
				t.Errorf("the repair reopened %d reminders, want none", got)
			}
			if !e.reminderIsDone(t, reminder) {
				t.Error("the repair reopened a reminder no newsletter closed")
			}
		})
	}
}

// The engine's own door already refuses a newsletter, so nothing new needs
// the repair.
func TestANewsletterAnswersNoQuietReminder(t *testing.T) {
	e := setupRecency(t)
	deal := e.seedQuietDeal(t, pinnedQuiet)
	reminder := e.seedQuietReminder(t, deal, pinnedQuiet)
	mail := e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, 89))
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	closed, err := store.CompleteQuietRemindersReachedBy(e.asResolver(), ids.From[ids.ActivityKind](mail))
	if err != nil {
		t.Fatal(err)
	}
	if closed != 0 || e.reminderIsDone(t, reminder) {
		t.Errorf("a newsletter closed %d reminders, want none", closed)
	}
}

func TestANewsletterMovesNoDealClock(t *testing.T) {
	e := setupRecency(t)
	deal := e.seedQuietDeal(t, pinnedQuiet)
	e.seedNewsletter(t, deal, pinnedQuiet.AddDate(0, 0, 89))
	if at := e.lastActivity(t, deal); at == nil || !at.Equal(pinnedQuiet) {
		t.Errorf("deal last_activity_at = %v, want the buyer's message at %v", at, pinnedQuiet)
	}
}

// A newsletter reaches an account's reminder through the contact it employs,
// the walk the engine's completion took.
func TestTheRepairReopensAnAccountReminderANewsletterReachedThroughItsContact(t *testing.T) {
	e := setupRecency(t)
	company, contact, reminder := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Quiet Co', 'manual', 'human:seed')`, company)
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Quiet Buyer', 'manual', 'human:seed')`, contact)
	e.exec(t, `INSERT INTO relationship (kind, contact_id, company_id, role, source, captured_by)
		VALUES ('employment', $1, $2, 'Buyer', 'manual', 'human:seed')`, contact, company)
	sourceID := "no_activity_reminder:company:" + company.String() + provenance.ReminderAnchorSeparator + pinnedQuiet.Format(time.RFC3339Nano)
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, audience, source, captured_by, source_system, source_id, is_done, origin)
		VALUES ($1, 'task', 'No activity for a while', now(), 'workspace', 'system', 'system:time-scan', 'no_activity_reminder', $2, false, 'system_remediation')`,
		reminder, sourceID)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, company_id) VALUES ($1, $2, 'company', $3)`, ids.NewV7(), reminder, company)
	mail := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, audience, source, captured_by, origin, bulk_mail_attested)
		VALUES ($1, 'email', 'inbound', 'Our autumn newsletter', $2, 'workspace', 'gmail', 'connector:gmail', 'human', true)`,
		mail, pinnedQuiet.AddDate(0, 0, 89))
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), mail, contact)
	e.closeAs(e.asEngine(), t, reminder)

	if got := e.repairNewsletterReminders(t); got != 1 || e.reminderIsDone(t, reminder) {
		t.Errorf("the repair reopened %d account reminders, want the one the newsletter closed", got)
	}
}
