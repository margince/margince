// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An account's direction clocks say how precisely each is known, from the
// activity behind it, on every surface that prints one.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// logDirected files a call with the account's contact, known by its date alone
// or at an instant.
func logDirected(t *testing.T, e *integration.Env, acct quietAccount, direction string, at time.Time, precision string) {
	t.Helper()
	subject := "Call with the account"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "call", Subject: &subject, Direction: &direction, OccurredAt: &at, OccurredPrecision: precision,
		Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: acct.contact}},
	}); err != nil {
		t.Fatalf("logging the %s call: %v", direction, err)
	}
}

func isDay(precision *crmcontracts.OccurredPrecision) bool {
	return precision != nil && occurredday.IsDay(*precision)
}

func TestAnAccountsDirectionClocksCarryTheirPrecision(t *testing.T) {
	e := integration.Setup(t)
	acct := employedAccount(t, e, "Dated Calls GmbH")
	inbound := occurredday.AnchorIn(acct.now.AddDate(0, 0, -3), time.UTC)
	outbound := acct.now.Add(-time.Hour).Truncate(time.Microsecond)
	logDirected(t, e, acct, "inbound", inbound, activities.PrecisionDay)
	logDirected(t, e, acct, "outbound", outbound, activities.PrecisionInstant)

	page := pageOf(t, e, acct)
	strip := page.StateStrip.Engagement
	if strip == nil || !isDay(strip.LastInboundPrecision) || strip.LastOutboundPrecision == nil || isDay(strip.LastOutboundPrecision) {
		t.Errorf("the state strip reads %+v, want a day inbound and an instant outbound", strip)
	}

	service := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), func() time.Time { return acct.now })
	contactsPage, err := service.ContactPage(e.Admin(), acct.company, company360.ContactListQuery{})
	if err != nil || len(contactsPage.Data) != 1 {
		t.Fatalf("the contact list: %v, %d rows", err, len(contactsPage.Data))
	}
	row := contactsPage.Data[0]
	if !isDay(row.LastInboundPrecision) || row.LastOutboundPrecision == nil || isDay(row.LastOutboundPrecision) {
		t.Errorf("the contact row reads in %v out %v, want a day inbound and an instant outbound",
			row.LastInboundPrecision, row.LastOutboundPrecision)
	}
	if !row.LastInboundAt.Equal(inbound) {
		t.Errorf("the contact row's last inbound is %v, want the day's anchor %v", row.LastInboundAt, inbound)
	}
	coverage, err := service.Coverage(e.Admin(), acct.company)
	if err != nil {
		t.Fatal(err)
	}
	if route := coverage.BestWayIn; route == nil || !isDay(route.LastInboundPrecision) {
		t.Errorf("the best way in is %+v, want it to read a day inbound", route)
	}
}
