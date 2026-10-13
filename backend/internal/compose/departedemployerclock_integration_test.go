// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// companyClockRow is what a refold may change, plus xmin, which moves on any
// write at all.
type companyClockRow struct {
	lastActivity string
	version      int
	xmin         string
}

func readCompanyClockRow(t *testing.T, owner *pgx.Conn, company ids.UUID) companyClockRow {
	t.Helper()
	var row companyClockRow
	if err := owner.QueryRow(context.Background(),
		`SELECT coalesce(last_activity_at::text, ''), version, xmin::text FROM company WHERE id = $1`, company,
	).Scan(&row.lastActivity, &row.version, &row.xmin); err != nil {
		t.Fatalf("reading the company clock: %v", err)
	}
	return row
}

// seedDepartedEmployer mails a contact serving notice, then dates the departure
// `daysAgo` and puts the company clock back on the mail, as an ended notice leaves it.
func seedDepartedEmployer(t *testing.T, e *integration.Env, owner *pgx.Conn, name string, daysAgo int) (ids.UUID, time.Time) {
	t.Helper()
	ctx := context.Background()
	var today time.Time
	if err := owner.QueryRow(ctx, `SELECT current_date`).Scan(&today); err != nil {
		t.Fatalf("reading current_date: %v", err)
	}
	company := e.SeedCompany(t, name, &e.Rep1)
	contact := e.SeedContact(t, "Ann at "+name, &e.Rep1)
	contactID, companyID := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	notice := today.AddDate(0, 0, 30)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Source: "manual",
		EmploymentStatus: new("current"), EndedPrecision: new("day"), EndedAt: &notice,
	}); err != nil {
		t.Fatalf("recording the employment: %v", err)
	}
	mail := today.AddDate(0, 0, -daysAgo-3).Add(9 * time.Hour)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Call notes"), Source: "manual", OccurredAt: &mail,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatalf("logging the note: %v", err)
	}
	var clock *time.Time
	if err := owner.QueryRow(ctx, `SELECT last_activity_at FROM company WHERE id = $1`, company).Scan(&clock); err != nil {
		t.Fatalf("reading the company clock: %v", err)
	}
	if clock == nil || !clock.Equal(mail) {
		t.Fatalf("company clock = %v while the contact serves notice, want the mail %v", clock, mail)
	}

	if _, err := owner.Exec(ctx, `UPDATE relationship SET ended_at = current_date - $2::int WHERE company_id = $1`,
		company, daysAgo); err != nil {
		t.Fatalf("dating the departure: %v", err)
	}
	if _, err := owner.Exec(ctx, `UPDATE company SET last_activity_at = $2 WHERE id = $1`, company, mail); err != nil {
		t.Fatalf("staling the company clock: %v", err)
	}
	return company, mail
}

func runDepartedEmployerRefold(t *testing.T, e *integration.Env) {
	t.Helper()
	w := &departedEmployerClockWorker{
		pool: e.Pool, identity: identity.NewService(e.Pool),
		log: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	if err := w.Work(context.Background(), &river.Job[DepartedEmployerClockRefoldArgs]{}); err != nil {
		t.Fatalf("running the refold: %v", err)
	}
}

func TestTheRefoldMovesAClockADepartureTodayLeftStaleAndThenWritesNothing(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	company, _ := seedDepartedEmployer(t, e, owner, "Leaving Today", 0)
	stale := readCompanyClockRow(t, owner, company)

	runDepartedEmployerRefold(t, e)
	refolded := readCompanyClockRow(t, owner, company)
	if refolded.lastActivity != "" {
		t.Fatalf("company clock = %v after the refold, want none: the only mail went to a contact who has left", refolded.lastActivity)
	}
	if refolded.version != stale.version {
		t.Errorf("version moved %d -> %d: a clock refold is not an edit", stale.version, refolded.version)
	}

	runDepartedEmployerRefold(t, e)
	if again := readCompanyClockRow(t, owner, company); again != refolded {
		t.Errorf("the second pass wrote the company again (%+v -> %+v), want no write", refolded, again)
	}
}

func TestTheRefoldLeavesADepartureOutsideItsLookbackAlone(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	company, mail := seedDepartedEmployer(t, e, owner, "Left Last Month", 30)
	stale := readCompanyClockRow(t, owner, company)

	runDepartedEmployerRefold(t, e)
	after := readCompanyClockRow(t, owner, company)
	if after != stale {
		t.Errorf("a departure 30 days ago was refolded (%+v -> %+v), want the clock left on %v", stale, after, mail)
	}
}
