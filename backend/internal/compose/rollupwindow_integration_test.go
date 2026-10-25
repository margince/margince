// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// The account's 30-day count holds the thirty dates up to today. A day call
// dated today, the window's last date, counts though its stored noon is still
// ahead east of UTC+12. The date before the window does not, nor tomorrow's,
// nor a call at the instant read.
func TestADayCallCountsInTheThirtyDaysItsDateFallsIn(t *testing.T) {
	for _, zone := range []string{"Pacific/Auckland", "Pacific/Kiritimati"} {
		t.Run(zone, func(t *testing.T) {
			e := integration.Setup(t)
			loc := setInstallationZone(t, e, zone)
			now := databaseNow(t, e)
			today := occurredday.AnchorIn(now, loc)
			inside, buyer := logCallsOn(t, e, "Inside Freight", today, today.AddDate(0, 0, -29))
			outside, _ := logCallsOn(t, e, "Outside Freight", today.AddDate(0, 0, -30), today.AddDate(0, 0, 1))
			// The window is half open: a call at the very instant read is not in it yet.
			for subject, at := range map[string]time.Time{"Called an hour ago": now.Add(-time.Hour), "Calling now": now} {
				if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
					Kind: "call", Subject: &subject, Source: "manual", OccurredAt: &at,
					Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
				}); err != nil {
					t.Fatal(err)
				}
			}

			for company, want := range map[ids.UUID]int{inside: 3, outside: 0} {
				err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
					got, err := companyActivityCount30d(e.Admin(), tx, []ids.UUID{company}, now)
					if err == nil && got != want {
						t.Errorf("%d activities in the 30 days, want %d", got, want)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// logCallsOn logs a day call for each date with a buyer a new account employs.
// It returns the account and the buyer.
func logCallsOn(t *testing.T, e *integration.Env, name string, dates ...time.Time) (company, buyer ids.UUID) {
	t.Helper()
	company = e.SeedCompany(t, name, nil)
	buyer = e.SeedContact(t, name+" Buyer", nil)
	employee, employer := ids.From[ids.ContactKind](buyer), ids.From[ids.CompanyKind](company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &employee, CompanyID: &employer,
	}); err != nil {
		t.Fatalf("employing the buyer: %v", err)
	}
	for _, date := range dates {
		subject := "Called on " + date.Format(time.DateOnly)
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "call", Subject: &subject, Source: "manual", OccurredAt: &date, OccurredPrecision: activities.PrecisionDay,
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
		}); err != nil {
			t.Fatalf("logging a call on %s: %v", date.Format(time.DateOnly), err)
		}
	}
	return company, buyer
}
