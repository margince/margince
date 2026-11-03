// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The account's last-message dates count what the reader can open, as its
// contacts' do. Casey works at Northwind. Alex can open Casey's private reply
// and Blair cannot, so the reply answers Alex's account and leaves Blair's as
// it was.

// accountFixture is the contact fixture with Casey employed at an account.
type accountFixture struct {
	correspondenceFixture
	account ids.UUID
}

func newAccountFixture(t *testing.T) accountFixture {
	t.Helper()
	f := newCorrespondenceFixture(t)
	account := f.e.SeedCompany(t, "Northwind Freight", &f.e.Rep1)
	casey, company := ContactIDOf(f.casey), companyIDOf(account)
	if _, err := f.e.Contacts.CreateRelationship(f.e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &casey, CompanyID: &company,
	}); err != nil {
		t.Fatalf("employing Casey at the account: %v", err)
	}
	return accountFixture{correspondenceFixture: f, account: account}
}

func (f accountFixture) accountPage(reader context.Context, t *testing.T) crmcontracts.Company360 {
	t.Helper()
	page, err := companySurfaceService(f.e).Assemble(reader, companyIDOf(f.account))
	if err != nil {
		t.Fatalf("assembling the account page: %v", err)
	}
	return page
}

// accountWorklistTouch is the reading the account Worklist row asks for
// (attentiontouchseam.go).
func (f accountFixture) accountWorklistTouch(reader context.Context, t *testing.T) company360.LastTouch {
	t.Helper()
	var touch company360.LastTouch
	if err := database.WithWorkspaceTx(reader, f.e.Pool, func(tx pgx.Tx) error {
		touched, err := company360.LastTouchFor(reader, tx, []ids.CompanyID{companyIDOf(f.account)}, roomFixedNow, company360.AssembleOptions{})
		touch = touched[companyIDOf(f.account)]
		return err
	}); err != nil {
		t.Fatalf("reading the account Worklist touch: %v", err)
	}
	return touch
}

func TestAReplyTheReaderCanOpenMovesTheirAccountsLastInbound(t *testing.T) {
	f := newAccountFixture(t)
	f.caseyReplies(t)
	replyAt := roomAgo(2 * 24 * time.Hour)

	page := f.accountPage(f.alex, t)
	if page.LastInboundAt == nil || !page.LastInboundAt.Equal(replyAt) {
		t.Fatalf("Alex's account last_inbound_at = %v, want Casey's reply at %v", page.LastInboundAt, replyAt)
	}
	if touch := f.accountWorklistTouch(f.alex, t); touch.InboundAt == nil || !touch.InboundAt.Equal(replyAt) {
		t.Errorf("Alex's account Worklist inbound = %v, want the reply the account page shows", touch.InboundAt)
	}
	if state := accountdraft.ConversationState(page, roomFixedNow, time.UTC); state.LastDirection != convstate.DirectionInbound {
		t.Errorf("Alex's account draft reads the last word as %q, want Casey's readable reply", state.LastDirection)
	}
}

func TestAReplyTheReaderCannotOpenLeavesTheirAccountAsItWas(t *testing.T) {
	f := newAccountFixture(t)
	before := f.accountPage(f.blair, t)
	f.caseyReplies(t)
	after := f.accountPage(f.blair, t)

	if !sameInstant(after.LastInboundAt, before.LastInboundAt) || !sameInstant(after.LastOutboundAt, before.LastOutboundAt) {
		t.Errorf("Blair's account moved from %v/%v to %v/%v on a reply Blair cannot open",
			before.LastInboundAt, before.LastOutboundAt, after.LastInboundAt, after.LastOutboundAt)
	}
	if touch := f.accountWorklistTouch(f.blair, t); touch.InboundAt != nil {
		t.Errorf("Blair's account Worklist inbound = %v, want none", touch.InboundAt)
	}
}

func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
