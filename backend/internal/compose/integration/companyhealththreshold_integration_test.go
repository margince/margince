// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The company page's single-threaded verdict follows the threshold an admin
// publishes for Account coverage, the rule the deal page reads too.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
)

func TestTheCompanyPageFollowsThePublishedThreadingThreshold(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	f := seedTwoEngagementAccount(t, e)
	employAtAccount(t, e, f)
	companyID := companyIDOf(f.company)
	// A meeting already held with a second contact gives the account two active contacts.
	held, subject := roomFixedNow.AddDate(0, 0, -6), "Kick-off"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", MeetingStatus: new("booked"), Subject: &subject, OccurredAt: &held,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: f.contact}, {EntityType: "contact", EntityID: f.attendee}},
	}); err != nil {
		t.Fatalf("log the held meeting: %v", err)
	}
	health := func() *crmcontracts.Company360Health {
		t.Helper()
		page, err := companySurfaceService(e).AssembleScoped(e.Admin(), companyID, company360.AssembleOptions{})
		if err != nil || page.Health == nil || page.Health.ActiveContacts == nil || page.Health.SingleThreaded == nil {
			t.Fatalf("assembling the company page: %v, health %+v", err, page.Health)
		}
		return page.Health
	}
	if got := health(); *got.ActiveContacts != 2 || *got.SingleThreaded {
		t.Fatalf("at the default threshold of one, %d active contacts read single-threaded %v, want 2 and false",
			*got.ActiveContacts, *got.SingleThreaded)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO reporting_framework_revision (revision, definition, effective_at, created_by)
			VALUES (1, '{"template":"sales","qualification":[],"capture_contexts":[],"reason":"test","account_coverage":{"max_engaged_seats":2}}', now(), $1)`, e.AdminUser)
		if err != nil {
			return err
		}
		_, err = tx.Exec(context.Background(), `INSERT INTO reporting_framework (revision, version) VALUES (1, 1)`)
		return err
	}); err != nil {
		t.Fatalf("publishing a threshold of two: %v", err)
	}
	if got := health(); !*got.SingleThreaded {
		t.Fatalf("at a threshold of two, %d active contacts read single-threaded false, want true", *got.ActiveContacts)
	}
}
