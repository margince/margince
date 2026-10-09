// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A deal stakeholder may be recorded without a role — the column is nullable
// and the contacts store accepts a seat with none. The contact page's
// commercial band read that role into a plain string, so a contact seated on
// an open deal without one failed the whole page (and the brief built on it)
// with a scan error instead of rendering.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAStakeholderWithoutARoleStillRendersTheCommercialBand(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Rollout without roles", PipelineID: pipeline, StageID: stage, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))

	// Both the page's own contact and the colleague beside them are seated
	// with no role: the first exercises the leading seat, the second the
	// committee row.
	seatWithoutRole := func(name string) ids.ContactID {
		t.Helper()
		contact := ids.From[ids.ContactKind](e.SeedContact(t, name, &e.AdminUser))
		if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
			Kind: "deal_stakeholder", ContactID: &contact, DealID: &dealID, Source: "manual",
		}); err != nil {
			t.Fatalf("seating %s: %v", name, err)
		}
		return contact
	}
	reader := seatWithoutRole("Ohne Rolle")
	colleague := seatWithoutRole("Auch Ohne Rolle")

	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), time.Now)

	page, err := svc.Assemble(e.Admin(), reader)
	if err != nil {
		t.Fatalf("assembling contact360 for a seat with no role: %v", err)
	}
	if page.Commercial == nil || page.Commercial.Deal == nil {
		t.Fatalf("the commercial band is %+v, want the open deal this contact sits on", page.Commercial)
	}
	if page.Commercial.Role != nil {
		t.Errorf("role = %q, want none — the seat was recorded without one, and naming one invents it",
			*page.Commercial.Role)
	}
	if len(page.Commercial.Committee) != 1 {
		t.Fatalf("committee carries %d members, want the one colleague", len(page.Commercial.Committee))
	}
	member := page.Commercial.Committee[0]
	if ids.UUID(member.ContactId) != colleague.UUID {
		t.Errorf("committee member = %v, want the colleague %v", member.ContactId, colleague)
	}
	if member.Role != "" {
		t.Errorf("committee role = %q, want empty for a seat recorded without one", member.Role)
	}
}
