// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package companyscan

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// A canceled meeting is no exchange, so the model never reads it as one and it
// takes no slot from real correspondence in the window.
func TestACanceledMeetingIsNotAnExchangeTheScanReads(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	company := e.SeedCompany(t, "Fold Scan Co", nil)
	contact := e.SeedContact(t, "Sky Scan", nil)
	contactID, companyID, primary := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company), true
	if _, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: &primary,
	}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
	onAccount := []datasource.EntityRef{{Type: "contact", ID: contact}}
	now := time.Now().UTC()
	held := integration.CalendarMeeting{Event: "evt-scan-held", At: now.AddDate(0, 0, -10), Links: onAccount}
	called := integration.CalendarMeeting{Event: "evt-scan-called", At: now.AddDate(0, 0, -2), Links: onAccount}
	heldID, calledID := held.Capture(t, e), called.Capture(t, e)

	read := func() []ids.UUID {
		var words []MessageIn
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			var err error
			words, err = readWords(ctx, tx, companyID, now)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		out := make([]ids.UUID, 0, len(words))
		for _, w := range words {
			out = append(out, w.ID)
		}
		return out
	}
	if got := read(); !slices.Contains(got, calledID) {
		t.Fatalf("the booked meeting is not read before the cancel (%v) — the case below proves nothing", got)
	}
	called.Cancel(t, e)
	got := read()
	if slices.Contains(got, calledID) {
		t.Error("the scan reads a canceled meeting as an exchange")
	}
	if !slices.Contains(got, heldID) {
		t.Errorf("the scan lost the held meeting: %v", got)
	}
}
