// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The review findings on Lists, each held against Postgres: a why never names a
// record its reader cannot open, a masked or refused read still answers rather
// than failing, a membership change needs the record's own read grant, and an
// erasure reaches every lead it anonymizes.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealListPerms reads and changes deals, reads companies and works lists,
// team-scoped.
func dealListPerms() principal.Permissions {
	p := RepPerms
	p.Objects = map[string]principal.ObjectGrant{
		"deal":    {Create: true, Read: true, Update: true},
		"company": {Read: true},
		"list":    {Create: true, Read: true, Update: true, Delete: true},
	}
	return p
}

// dealWithHiddenCompany seeds a deal whose company is capture-private to Rep1,
// and a workspace Live List selecting deals that have a company.
func dealWithHiddenCompany(t *testing.T, e *Env) (collections.CreateListInput, ids.UUID, ids.UUID) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Hidden Account", &e.Rep1)
	e.MakeCapturePrivate(t, "company", company, e.Rep1)
	companyID := ids.From[ids.CompanyKind](company)
	// Created by the capturer, the one seat that may name the company.
	deal, err := e.Deals.CreateDeal(e.As(e.Rep1, []ids.UUID{e.Team1}, dealListPerms()), deals.CreateDealInput{
		Name: "Deal on a hidden account", PipelineID: pipeline, StageID: open, CompanyID: &companyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return collections.CreateListInput{
		Name: "Deals with an account", EntityType: "deal", ListType: "dynamic", Sharing: "workspace",
		Definition: map[string]any{"field": "company_id", "op": "exists", "value": true},
	}, ids.UUID(deal.Id), company
}

func TestAWhyNeverNamesARecordItsReaderCannotOpen(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	input, deal, company := dealWithHiddenCompany(t, e)
	list, err := store.CreateList(e.Admin(), input)
	if err != nil {
		t.Fatal(err)
	}
	outsider := e.As(e.Rep3, []ids.UUID{e.Team2}, dealListPerms())
	why, err := store.ExplainMember(outsider, list.ID, deal)
	if err != nil {
		t.Fatalf("explain for a reader who may see the deal: %v", err)
	}
	if !why.Clauses.Hidden || why.Clauses.Value != nil {
		t.Fatalf("a reader who cannot open the company was shown %+v", why.Clauses)
	}
	capturer := e.As(e.Rep1, []ids.UUID{e.Team1}, dealListPerms())
	seen, err := store.ExplainMember(capturer, list.ID, deal)
	if err != nil || seen.Clauses.Value == nil || *seen.Clauses.Value != company.String() {
		t.Fatalf("the capturer was shown %+v (%v), want the company", seen.Clauses, err)
	}
}

func TestAMaskedFieldStillExplainsForAScopedWriter(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	input, deal, _ := dealWithHiddenCompany(t, e)
	list, err := store.CreateList(e.Admin(), input)
	if err != nil {
		t.Fatal(err)
	}
	masked := dealListPerms()
	masked.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "company_id", Condition: principal.MaskOutsideWriteAuthority}}
	why, err := store.ExplainMember(e.As(e.Rep3, []ids.UUID{e.Team2}, masked), list.ID, deal)
	if err != nil {
		t.Fatalf("a masked field broke the explanation: %v", err)
	}
	if !why.Clauses.Hidden {
		t.Fatalf("a masked value was shown: %+v", why.Clauses)
	}
}

func TestAReaderRefusedTheRecordTypeReadsAnEmptyHistory(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	contact := e.SeedContact(t, "Listed Contact", &e.Rep1)
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{Name: "Open picks", EntityType: "contact", Sharing: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMember(e.Admin(), list.ID, collections.MemberChange{EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen}); err != nil {
		t.Fatal(err)
	}
	listOnly := RepPerms
	listOnly.Objects = map[string]principal.ObjectGrant{"list": {Read: true}}
	history, _, err := store.History(e.As(e.Rep3, []ids.UUID{e.Team2}, listOnly), list.ID, 10, "")
	if err != nil {
		t.Fatalf("history for a reader refused contacts: %v", err)
	}
	for _, entry := range history {
		if entry.EntityID != nil {
			t.Fatalf("a reader refused contacts read a membership change: %+v", entry)
		}
	}
}

func TestAddingToAShortlistNeedsTheRecordsReadGrant(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	contact := e.SeedContact(t, "Unreadable To The Curator", &e.Rep1)
	curator := RepPerms
	curator.Objects = map[string]principal.ObjectGrant{"list": {Create: true, Read: true, Update: true}}
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, curator)
	list, err := store.CreateList(ctx, collections.CreateListInput{Name: "Mine", EntityType: "contact"})
	if err != nil {
		t.Fatal(err)
	}
	change := collections.MemberChange{EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen}
	if _, err := store.AddMember(ctx, list.ID, change); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a curator without contact read added a contact: %v, want not found", err)
	}
	if _, err := store.RemoveMember(ctx, list.ID, change); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a curator without contact read removed a contact: %v, want not found", err)
	}
}

func TestErasureTakesEveryAnonymizedLeadOffItsShortlists(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	const address = "listed-lead@erasure.example"
	name := "Listed Lead"
	email := address
	// The contact first, so the lead is a separate row matched to them by
	// address alone: creating the contact after would take the lead with it.
	subject, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Twin Subject", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Email: &email, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND promoted_contact_id IS NULL`, lead.Id); n != 1 {
		t.Fatal("the lead was promoted, so this test would not reach a lead matched by address alone")
	}
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{Name: "Lead picks", EntityType: "lead"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMember(e.Admin(), list.ID, collections.MemberChange{
		EntityType: "lead", EntityID: ids.UUID(lead.Id), Reason: collections.ReasonChosen,
	}); err != nil {
		t.Fatal(err)
	}
	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), ids.UUID(subject.Id), "test"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM list_member WHERE entity_id = $1`, lead.Id); n != 0 {
		t.Fatalf("the anonymized lead is still on %d Shortlists", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM list_member_event WHERE entity_id = $1`, lead.Id); n != 0 {
		t.Fatalf("%d membership events of the anonymized lead outlived the erasure", n)
	}
}
