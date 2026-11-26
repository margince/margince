// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// The account's committee lists the seats still current, and its total counts
// the same seats. Counting an ended seat in the total alone would report it as
// a seat the reader may not see.
func TestTheAccountCommitteeCountsOnlyCurrentSeats(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	company := e.SeedCompany(t, "Brandt GmbH", nil)
	deal := ids.From[ids.DealKind](e.SeedOpenDealOn(t, "Fleet renewal", company))
	today, err := time.Parse(time.DateOnly, e.WsScalar(t, `SELECT (`+occurredday.TodaySQL("now()")+`)::text`))
	if err != nil {
		t.Fatalf("reading the installation's today: %v", err)
	}
	yesterday, tomorrow := today.AddDate(0, 0, -1), today.AddDate(0, 0, 1)
	for name, end := range map[string]*time.Time{
		"Open-ended": nil, "Ended yesterday": &yesterday, "Ends today": &today, "Ends tomorrow": &tomorrow,
	} {
		contact := ids.From[ids.ContactKind](e.SeedContact(t, name, nil))
		employ(t, e, contact.UUID, company, "Buyer")
		edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{
			Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Source: "manual",
		})
		if err != nil {
			t.Fatalf("seating %s: %v", name, err)
		}
		if end == nil {
			continue
		}
		if _, err := e.Contacts.UpdateRelationship(ctx, edge.ID, contacts.UpdateRelationshipInput{EndedAt: end}); err != nil {
			t.Fatalf("ending %s's seat: %v", name, err)
		}
	}

	got, err := company360Service(e).Coverage(ctx, ids.CompanyID{UUID: company}, nil)
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	if got.Committee == nil {
		t.Fatal("the coverage read no committee on an account with an open deal")
	}
	if len(got.Committee.Seats) != 2 || got.Committee.UnlistedSeats != 0 {
		t.Fatalf("the committee lists %d seats and %d unlisted, want 2 and 0",
			len(got.Committee.Seats), got.Committee.UnlistedSeats)
	}
}
