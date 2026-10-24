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
	"github.com/margince/margince/backend/internal/platform/clocktest"
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

// The scan reads a reply as what its sender wrote, so a quoted mail under it is
// neither shown to the model nor quotable as the reply's words.
func TestTheScanReadsOnlyWhatTheSenderWrote(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	company := e.SeedCompany(t, "Quote Scan Co", nil)
	mail := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id, source, captured_by)
		VALUES ($1, 'email', 'next steps', $2, 'inbound', 'gmail', $3, 'gmail:seed', 'connector:gmail')`, mail,
		"Works for me.\n\nFrom: Sam Seller <sam@seller.example>\nSent: Monday\n\nWe will send the contract by Friday.",
		mail.String())
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`, mail, company)

	var words []MessageIn
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		words, err = readWords(ctx, tx, ids.From[ids.CompanyKind](company), clocktest.Now(t))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(words) != 1 || words[0].Text != "Works for me." {
		t.Fatalf("the scan read %+v, want the one reply holding only its sender's line", words)
	}
}
