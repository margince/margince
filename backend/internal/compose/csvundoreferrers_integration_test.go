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
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An undo archives the companies a run created. A live deal or a contact's
// employment that still names one must not be left pointing at it.
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
		Kind: relationshipKindEmployment, ContactID: &contact, CompanyID: &companyID, IsCurrentPrimary: new(true), Source: "manual",
	}); err != nil {
		t.Fatalf("seeding the employment: %v", err)
	}
	w := &csvWriters{pool: e.DB().Pool(), contacts: e.Contacts, deals: e.Deals}
	importedAt := clocktest.Now(t).Add(time.Hour)
	reverse := func() error { return w.Reverse(e.Admin(), migration.ObjectCompany, company, importedAt) }
	live := func() bool {
		c, err := e.Contacts.GetCompany(e.Admin(), companyID, storekit.IncludeArchived)
		return err == nil && c.ArchivedAt == nil
	}

	if err := reverse(); !keptStillReferenced(err) || !live() {
		t.Fatalf("reverse with a live deal and contact = %v, company live %v; want it kept as still referenced", err, live())
	}
	if _, err := e.Deals.ArchiveDeal(e.Admin(), ids.From[ids.DealKind](ids.UUID(deal.Id)), nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	if err := reverse(); !keptStillReferenced(err) || !live() {
		t.Fatalf("reverse with a live employment = %v, company live %v; want it kept as still referenced", err, live())
	}
	if _, err := e.Contacts.ArchiveContact(e.Admin(), contact, nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if err := reverse(); err != nil || live() {
		t.Fatalf("reverse with nothing live = %v, company live %v; want it archived", err, live())
	}
}

func keptStillReferenced(err error) bool {
	kept, ok := errors.AsType[*migration.KeptError](err)
	return ok && kept.Why.Code == migration.ReasonUndoStillReferenced
}
