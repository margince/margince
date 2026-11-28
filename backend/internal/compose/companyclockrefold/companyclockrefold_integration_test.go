// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package companyclockrefold

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// account is a company whose one employee wrote mail two days ago, so the
// company's clock reads that mail.
type account struct {
	e          *integration.Env
	company    ids.UUID
	employment ids.UUID
	mailAt     time.Time
}

func newAccount(t *testing.T) account {
	t.Helper()
	e := integration.Setup(t)
	a := account{e: e, company: e.SeedCompany(t, "Notice Co", nil), mailAt: clocktest.Now(t).Add(-48 * time.Hour).Truncate(time.Second)}
	contact := e.SeedContact(t, "Nora Notice", nil)
	contactID, companyID, primary := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](a.company), true
	edge, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: &primary,
	})
	if err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
	a.employment = edge.ID
	direction, subject := "inbound", "Proposal"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &a.mailAt, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	if got := a.clock(t); got == nil || !got.Equal(a.mailAt) {
		t.Fatalf("company clock = %v, want the employee's mail at %v", got, a.mailAt)
	}
	return a
}

func (a account) clock(t *testing.T) *time.Time {
	t.Helper()
	var got *time.Time
	if err := database.WithTx(a.e.Admin(), a.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(a.e.Admin(), `SELECT last_activity_at FROM company WHERE id = $1`, a.company).Scan(&got)
	}); err != nil {
		t.Fatalf("reading the company clock: %v", err)
	}
	return got
}

// endYesterday ends the employment through the writer, which refolds the
// clock. It then puts the stale clock back. That is the state a departure day
// leaves when it arrives with no write.
func (a account) endYesterday(t *testing.T, status *string) {
	t.Helper()
	yesterday := clocktest.Now(t).Add(-24 * time.Hour).Truncate(24 * time.Hour)
	if _, err := a.e.Contacts.UpdateRelationship(a.e.Admin(), a.employment,
		contacts.UpdateRelationshipInput{EmploymentStatus: status, EndedAt: &yesterday}); err != nil {
		t.Fatalf("ending the employment: %v", err)
	}
	a.e.WsExec(t, `UPDATE company SET last_activity_at = $1 WHERE id = $2`, a.mailAt, a.company)
}

// A departure day arrives and no row changes. The refold finds the company by
// the end day alone, and only inside the window it is asked about.
func TestACompanyClockDropsAnEmployeeWhoseDepartureDayArrivedWithoutAWrite(t *testing.T) {
	a := newAccount(t)
	a.endYesterday(t, nil)
	ctx := principal.SystemActing(a.e.Admin(), "system:clock-refold-test")
	db := database.New(a.e.Pool)
	now := clocktest.Now(t)

	if n, err := RefoldEnded(ctx, db, now.Add(-time.Hour), now); err != nil || n != 0 {
		t.Fatalf("a window after the departure refolded %d companies (%v), want 0", n, err)
	}
	if got := a.clock(t); got == nil || !got.Equal(a.mailAt) {
		t.Fatalf("a departure before the window was read again, clock = %v", got)
	}
	if n, err := RefoldEnded(ctx, db, now.Add(-72*time.Hour), now); err != nil || n != 1 {
		t.Fatalf("the window holding the departure refolded %d companies (%v), want 1", n, err)
	}
	if got := a.clock(t); got != nil {
		t.Errorf("the company still counts mail from an employee gone since yesterday, clock = %v", got)
	}
}

// A clock stored under the old predicate is refolded once after the upgrade.
func TestTheUpgradeTaskRefoldsACompanyClockStoredUnderTheOldPredicate(t *testing.T) {
	a := newAccount(t)
	former := "former"
	a.endYesterday(t, &former)

	task := DataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	now := clocktest.Now(t)
	engine, err := datatask.New(a.e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	ctx := principal.SystemActing(a.e.Admin(), "system:datatask-test")
	if err == nil {
		err = engine.Seed(ctx)
	}
	for i := 0; err == nil && i < 40; i++ {
		_, err = engine.Execute(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := a.clock(t); got != nil {
		t.Errorf("the upgrade task left the clock at %v, want none", got)
	}
}

// An install that finished version 1 runs the task again after the migration
// that moved "today" to the installation zone. The engine treats a step stored
// at an older version as unfinished.
func TestAnInstallThatFinishedVersionOneRefoldsAfterTheNewMigration(t *testing.T) {
	a := newAccount(t)
	now := clocktest.Now(t)
	run := func(task datatask.Task) {
		t.Helper()
		task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
		engine, err := datatask.New(a.e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
			func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
		ctx := principal.SystemActing(a.e.Admin(), "system:datatask-test")
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

	previous := DataTask()
	previous.Version = 1
	previous.Requires = []string{"core/1795101500"}
	run(previous)

	former := "former"
	a.endYesterday(t, &former)
	if a.clock(t) == nil {
		t.Fatal("the fixture has no stale clock to refold")
	}
	run(DataTask())
	if got := a.clock(t); got != nil {
		t.Errorf("the version-2 run left the clock at %v, want none", got)
	}
}
