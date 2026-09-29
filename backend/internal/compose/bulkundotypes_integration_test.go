// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Undoing a bulk archive of companies and of deals, each seeded through its
// own writers with what its archive takes down, and the owner an undo cannot
// hand back.

import (
	"fmt"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// taggedAndListed tags id and puts it on a fresh list of entityType, and
// answers the tag and the list.
func taggedAndListed(t *testing.T, e *integration.Env, entityType string, id ids.UUID) (tag, list ids.UUID) {
	t.Helper()
	store := collections.NewStore(e.DB())
	created, err := store.CreateTag(e.Admin(), "undo-"+ids.NewV7().String(), nil, nil)
	if err != nil {
		t.Fatalf("seeding a tag: %v", err)
	}
	listed, err := store.CreateList(e.Admin(), collections.CreateListInput{Name: "Undo " + ids.NewV7().String(), EntityType: entityType})
	if err != nil {
		t.Fatalf("seeding a list: %v", err)
	}
	if _, err := store.ApplyTag(e.Admin(), created.ID, entityType, id); err != nil {
		t.Fatalf("tagging %s: %v", id, err)
	}
	if _, err := store.AddMember(e.Admin(), listed.ID, collections.MemberChange{EntityType: entityType, EntityID: id, Reason: collections.ReasonChosen}); err != nil {
		t.Fatalf("listing %s: %v", id, err)
	}
	return created.ID.UUID, listed.ID.UUID
}

// assertBackWithTagAndList checks one restored record is live, tagged and listed.
func assertBackWithTagAndList(t *testing.T, e *integration.Env, entityType string, id, tag, list ids.UUID) {
	t.Helper()
	if n := e.WsCount(t, `SELECT count(*) FROM `+entityType+` WHERE id = $1 AND archived_at IS NULL`, id); n != 1 {
		t.Errorf("%s %s is still archived", entityType, id)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM taggable WHERE entity_type = $1 AND entity_id = $2 AND tag_id = $3`,
		entityType, id, tag); n != 1 {
		t.Errorf("%s %s came back without its tag", entityType, id)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM list_member WHERE entity_type = $1 AND entity_id = $2 AND list_id = $3`,
		entityType, id, list); n != 1 {
		t.Errorf("%s %s came back without its list membership", entityType, id)
	}
}

func TestUndoingABulkArchiveOfCompaniesBringsBackTheirDomainsTagsAndLists(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	type seeded struct{ id, tag, list ids.UUID }
	var companies []seeded
	var items []crmcontracts.BulkItem
	for i := range 2 {
		created, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
			DisplayName: fmt.Sprintf("Undo Company %d %s", i, ids.NewV7().String()[:8]),
			Domains:     []contacts.CompanyDomainInput{{Domain: fmt.Sprintf("undo-%d-%s.test", i, ids.NewV7().String()[:8]), IsPrimary: true}},
		})
		if err != nil {
			t.Fatalf("seeding company %d: %v", i, err)
		}
		id := ids.UUID(created.Id)
		tag, list := taggedAndListed(t, e, "company", id)
		companies = append(companies, seeded{id, tag, list})
		items = append(items, crmcontracts.BulkItem{Id: created.Id, Version: versionOf(t, e, "company", ids.UUID(created.Id))})
	}
	archived, err := engine.Execute(e.Admin(), bulkChange{
		recordType: crmcontracts.BulkRecordTypeCompany, verb: crmcontracts.BulkVerbArchive, items: items,
	})
	if err != nil || archived.Changed != 2 {
		t.Fatalf("archiving two companies → %+v, %v", archived, err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_domain WHERE company_id = $1 AND archived_at IS NULL`, companies[0].id); n != 0 {
		t.Fatal("the archive left the company's domain live")
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), "")
	if err != nil || undone.Changed != 2 || len(undone.Skipped) != 0 {
		t.Fatalf("Undo → %+v, %v; want both companies back", undone, err)
	}
	for _, c := range companies {
		assertBackWithTagAndList(t, e, "company", c.id, c.tag, c.list)
		if n := e.WsCount(t, `SELECT count(*) FROM company_domain WHERE company_id = $1 AND archived_at IS NULL`, c.id); n != 1 {
			t.Errorf("company %s came back without its domain", c.id)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND action = 'restore' AND entity_type = 'company'`,
		undone.BatchId); n != 2 {
		t.Errorf("%d company restore rows carry the undo's batch id, want two", n)
	}
}

func TestAnUndoLeavesACompanyWhoseDomainAnotherCompanyHoldsNow(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	domain := "taken-" + ids.NewV7().String()[:8] + ".test"
	created, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Domain Holder " + ids.NewV7().String()[:8],
		Domains:     []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the company: %v", err)
	}
	archived, err := engine.Execute(e.Admin(), bulkChange{
		recordType: crmcontracts.BulkRecordTypeCompany, verb: crmcontracts.BulkVerbArchive,
		items: []crmcontracts.BulkItem{{Id: created.Id, Version: versionOf(t, e, "company", ids.UUID(created.Id))}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "New Holder", Domains: []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}},
	}); err != nil {
		t.Fatalf("giving the domain to a new company: %v", err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), "")
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	assertReasons(t, "undo", skipReasons(undone.Skipped), map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		created.Id: crmcontracts.BulkSkipReasonValueTaken,
	})
}

func TestUndoingABulkArchiveOfDealsBringsBackTheirTagsListsAndStakeholders(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	pipeline, open, _ := integration.DealFixture(t, e)
	stakeholder := ids.From[ids.ContactKind](e.SeedContact(t, "Deal Stakeholder", nil))
	type seeded struct{ id, tag, list, link ids.UUID }
	var deals []seeded
	var items []crmcontracts.BulkItem
	for i := range 2 {
		id := e.SeedDeal(t, fmt.Sprintf("Undo Deal %d", i), pipeline, open, nil)
		tag, list := taggedAndListed(t, e, "deal", id)
		dealID := ids.From[ids.DealKind](id)
		link, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
			Kind: "deal_stakeholder", ContactID: &stakeholder, DealID: &dealID,
		})
		if err != nil {
			t.Fatalf("seeding a stakeholder on deal %d: %v", i, err)
		}
		deals = append(deals, seeded{id, tag, list, link.ID})
		items = append(items, crmcontracts.BulkItem{Id: openapi_types.UUID(id), Version: versionOf(t, e, "deal", id)})
	}
	archived, err := engine.Execute(e.Admin(), bulkChange{
		recordType: crmcontracts.BulkRecordTypeDeal, verb: crmcontracts.BulkVerbArchive, items: items,
	})
	if err != nil || archived.Changed != 2 {
		t.Fatalf("archiving two deals → %+v, %v", archived, err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), "")
	if err != nil || undone.Changed != 2 {
		t.Fatalf("Undo → %+v, %v; want both deals back", undone, err)
	}
	for _, d := range deals {
		assertBackWithTagAndList(t, e, "deal", d.id, d.tag, d.list)
		if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE id = $1 AND archived_at IS NULL`, d.link); n != 1 {
			t.Errorf("deal %s came back without its stakeholder", d.id)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'deal.restored'
		AND envelope#>>'{trace,audit_log_id}' IN (SELECT id::text FROM audit_log WHERE batch_id = $1)`, undone.BatchId); n != 2 {
		t.Errorf("%d deal.restored events trace to the undo's batch, want two", n)
	}
}

func TestAnUndoCannotHandBackARecordThatHadNoOwner(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	items := seedBulkContacts(t, e, e.Rep1, 1)
	unowned := ids.From[ids.ContactKind](ids.UUID(items[0].Id))
	if _, err := e.Contacts.UpdateContact(e.Admin(), unowned, contacts.UpdateContactInput{Clear: []string{"owner_id"}}); err != nil {
		t.Fatalf("clearing the owner: %v", err)
	}
	items[0].Version = versionOf(t, e, "contact", ids.UUID(items[0].Id))
	moved, err := engine.Execute(e.Admin(), reassignTo(e.Rep2, items))
	if err != nil || moved.Changed != 1 {
		t.Fatalf("reassigning the unowned contact → %+v, %v", moved, err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(moved.BatchId), "")
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	assertReasons(t, "undo", skipReasons(undone.Skipped), map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		items[0].Id: crmcontracts.BulkSkipReasonNoPreviousOwner,
	})
	if got := contactOwner(t, e, items[0].Id); got != e.Rep2.String() {
		t.Errorf("the contact is owned by %q after the undo, want it left with the new owner", got)
	}
}
