// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An undo archives the companies a run created, and a live deal or a live
// contact's employment that still names one must not be left pointing at an
// archived company.
func TestUndoKeepsACompanyWhileALiveRecordStillNamesIt(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	company := e.SeedCompany(t, "Referenced GmbH", nil)
	companyID := ids.From[ids.CompanyKind](company)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Kept deal", PipelineID: pipeline, StageID: stage, CompanyID: &companyID,
	})
	if err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Kept contact", nil))
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: relationshipKindEmployment, ContactID: &contact, CompanyID: &companyID, IsCurrentPrimary: ptrTrue(), Source: "manual",
	}); err != nil {
		t.Fatalf("seeding the employment: %v", err)
	}
	w := &csvWriters{pool: e.DB().Pool(), contacts: e.Contacts, deals: e.Deals}
	importedAt := time.Now().Add(time.Hour)
	reverse := func() error { return w.Reverse(e.Admin(), migration.ObjectCompany, company, importedAt) }
	live := func() bool {
		c, err := e.Contacts.GetCompany(e.Admin(), companyID, storekit.IncludeArchived)
		return err == nil && c.ArchivedAt == nil
	}

	if err := reverse(); !errors.Is(err, apperrors.ErrConflict) || !live() {
		t.Fatalf("reverse with a live deal and contact = %v, company live %v; want a conflict and the company kept", err, live())
	}
	if _, err := e.Deals.ArchiveDeal(e.Admin(), ids.From[ids.DealKind](ids.UUID(deal.Id)), nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	if err := reverse(); !errors.Is(err, apperrors.ErrConflict) || !live() {
		t.Fatalf("reverse with a live employment = %v, company live %v; want a conflict and the company kept", err, live())
	}
	if _, err := e.Contacts.ArchiveContact(e.Admin(), contact, nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if err := reverse(); err != nil || live() {
		t.Fatalf("reverse with nothing live = %v, company live %v; want it archived", err, live())
	}
}
