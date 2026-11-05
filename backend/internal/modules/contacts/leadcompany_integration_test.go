// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A lead's company link over a real Postgres. These cover the offer a read
// makes, and the confirm that writes the link and nothing else. They also
// cover what an edit of the company name and a promotion do to it.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// linker binds one user with the grants a link and the promotion after it use, at one row scope.
func (e *privacyEnv) linker(user ids.UUID, scope principal.RowScope) context.Context {
	return e.linkerWith(user, scope, map[string]principal.ObjectGrant{
		"contact":      {Create: true, Read: true, Update: true},
		"company":      {Create: true, Read: true, Update: true},
		"lead":         {Create: true, Read: true, Update: true},
		"relationship": {Create: true, Read: true, Update: true},
	})
}

func (e *privacyEnv) linkerWith(user ids.UUID, scope principal.RowScope, grants map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		TeamIDs: []ids.UUID{e.team},
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  grants,
			RowScope: scope,
		},
	})
}

func (e *privacyEnv) linkCompany(ctx context.Context, t *testing.T, name string) ids.CompanyID {
	t.Helper()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("create company %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

func (e *privacyEnv) linkLead(ctx context.Context, t *testing.T, owner ids.UUID, email, company string) ids.LeadID {
	t.Helper()
	ownerID := ids.From[ids.UserKind](owner)
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{
		FullName: new("Lena Lead"), Email: &email, CompanyName: &company, Title: new("Buyer"),
		OwnerID: &ownerID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

// contactsHolding names the live contacts that hold an address.
func (e *privacyEnv) contactsHolding(ctx context.Context, t *testing.T, email string) []ids.ContactID {
	t.Helper()
	var held []ids.ContactID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.id FROM contact c JOIN contact_email ce ON ce.contact_id = c.id
			 WHERE ce.email = $1 AND c.archived_at IS NULL ORDER BY c.id`, email)
		if err != nil {
			return err
		}
		held, err = pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
		return err
	}); err != nil {
		t.Fatalf("read contacts holding %s: %v", email, err)
	}
	return held
}

// employed answers whether a contact holds a live employment at a company.
func (e *privacyEnv) employed(ctx context.Context, t *testing.T, contact ids.ContactID, company ids.CompanyID) bool {
	t.Helper()
	var held bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM relationship WHERE contact_id = $1 AND company_id = $2
			                  AND `+employment.LiveSlotSQL("")+`)`,
			contact, company).Scan(&held)
	}); err != nil {
		t.Fatalf("read employment: %v", err)
	}
	return held
}

func (e *privacyEnv) linkAudits(ctx context.Context, t *testing.T, lead ids.LeadID) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'lead' AND entity_id = $1 AND after ? 'company_id'`, lead).Scan(&n)
	}); err != nil {
		t.Fatalf("count the lead's link audits: %v", err)
	}
	return n
}

// employments counts the live employment edges at a company.
func (e *privacyEnv) employments(ctx context.Context, t *testing.T, company ids.CompanyID) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM relationship WHERE company_id = $1 AND `+employment.LiveSlotSQL("")+``,
			company).Scan(&n)
	}); err != nil {
		t.Fatalf("count employments: %v", err)
	}
	return n
}

// eventTypesOf lists the type of every outbox event one request emitted.
func (e *privacyEnv) eventTypesOf(ctx context.Context, t *testing.T, correlation ids.UUID) []string {
	t.Helper()
	var types []string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT envelope->>'type' FROM event_outbox
			 WHERE envelope->'trace'->>'correlation_id' = $1::text ORDER BY seq`, correlation)
		if err != nil {
			return err
		}
		types, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatalf("read the request's events: %v", err)
	}
	return types
}

func TestLinkingALeadWritesOnlyTheCompany(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.linkCompany(ctx, t, "Acme Rocket GmbH")
	lead := e.linkLead(ctx, t, e.owner, "lena@acme-rocket.test", "  acme   ROCKET gmbh ")

	offered, err := e.store.GetLead(ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if offered.CompanyMatch == nil || ids.UUID(offered.CompanyMatch.Id) != company.UUID {
		t.Fatalf("company_match = %+v, want the company the name spells with other case and spacing", offered.CompanyMatch)
	}
	contactsBefore := e.contactsHolding(ctx, t, "lena@acme-rocket.test")
	audits, events, _ := e.leadTrail(ctx, t, lead)

	correlation := ids.NewV7()
	linked, err := e.store.LinkLeadCompany(principal.WithCorrelationID(ctx, correlation), lead, company)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if got := e.eventTypesOf(ctx, t, correlation); len(got) != 1 || got[0] != "lead.updated" {
		t.Errorf("the link emitted %v, want exactly one lead.updated", got)
	}
	if linked.CompanyId == nil || ids.UUID(*linked.CompanyId) != company.UUID {
		t.Errorf("company_id = %v, want %s", linked.CompanyId, company)
	}
	if linked.Status != offered.Status || linked.ArchivedAt != nil || linked.PromotedContactId != nil {
		t.Errorf("the link promoted the lead: status %s -> %s, archived %v", offered.Status, linked.Status, linked.ArchivedAt)
	}
	a, ev, _ := e.leadTrail(ctx, t, lead)
	if a != audits+1 || ev != events+1 || e.linkAudits(ctx, t, lead) != 1 {
		t.Errorf("the link wrote %d audit row(s) and %d lead.updated event(s), want 1 and 1", a-audits, ev-events)
	}
	if got := e.contactsHolding(ctx, t, "lena@acme-rocket.test"); len(got) != len(contactsBefore) {
		t.Errorf("contacts holding the lead's address: %d before the link, %d after", len(contactsBefore), len(got))
	}
	if n := e.employments(ctx, t, company); n != 0 {
		t.Errorf("employments at the linked company = %d, want 0", n)
	}
	again, err := e.store.GetLead(ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if again.CompanyMatch != nil {
		t.Error("a linked lead still offers a company match")
	}
}

func TestLinkingALeadLeavesTheContactItsEmailNamesAlone(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.linkCompany(ctx, t, "Boreal Freight")
	existing, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Lena Lead",
		Emails:   []ContactEmailInput{{Email: "lena@boreal.test", EmailType: "work", IsPrimary: true}},
		Source:   "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(existing.Id))
	ownerID := ids.From[ids.UserKind](e.owner)
	lead := seedHistoricalLeadWithEmail(ctx, t, e.store, CreateLeadInput{
		FullName: new("Lena Lead"), CompanyName: new("Boreal Freight"), Title: new("Buyer"),
		OwnerID: &ownerID, Source: "manual",
	}, "lena@boreal.test")
	before, err := e.store.GetContact(ctx, contact, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Fatalf("link: %v", err)
	}
	after, err := e.store.GetContact(ctx, contact, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if *after.Version != *before.Version {
		t.Errorf("the contact's version moved from %d to %d", *before.Version, *after.Version)
	}
	if e.employed(ctx, t, contact, company) {
		t.Error("the link gave the existing contact an employment at the company")
	}
}

func TestALinkNeedsOnlyLeadUpdateAndCompanyRead(t *testing.T) {
	e := setupCapturePrivacy(t)
	company := e.linkCompany(e.linker(e.owner, principal.RowScopeOwn), t, "Echo Freight")
	ctx := e.linkerWith(e.owner, principal.RowScopeOwn, map[string]principal.ObjectGrant{
		"lead":    {Create: true, Read: true, Update: true},
		"company": {Read: true},
	})
	lead := e.linkLead(ctx, t, e.owner, "eve@echo.test", "Echo Freight")

	linked, err := e.store.LinkLeadCompany(ctx, lead, company)
	if err != nil {
		t.Fatalf("link without contact or relationship grants: %v", err)
	}
	if linked.CompanyId == nil || ids.UUID(*linked.CompanyId) != company.UUID {
		t.Errorf("company_id = %v, want %s", linked.CompanyId, company)
	}
}

func TestACompanyOutsideTheReadersScopeIsNeitherOfferedNorLinked(t *testing.T) {
	e := setupCapturePrivacy(t)
	ownerCtx := e.linker(e.owner, principal.RowScopeOwn)
	hidden := e.linkCompany(ownerCtx, t, "Private Holdings")
	if _, err := e.store.UpdateCompany(ownerCtx, hidden, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("make the company private: %v", err)
	}
	teammateCtx := e.linker(e.teammate, principal.RowScopeTeam)
	lead := e.linkLead(teammateCtx, t, e.teammate, "pat@private.test", "Private Holdings")

	read, err := e.store.GetLead(teammateCtx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.CompanyMatch != nil {
		t.Errorf("a company the reader cannot open was offered: %+v", read.CompanyMatch)
	}
	if _, err := e.store.LinkLeadCompany(teammateCtx, lead, hidden); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("linking a hidden company = %v, want ErrNotFound", err)
	}
	if got := e.contactsHolding(ownerCtx, t, "pat@private.test"); len(got) != 0 {
		t.Errorf("a refused link still made %d contact(s)", len(got))
	}
}

func TestChangingTheCompanyNameUnlinksTheLead(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.linkCompany(ctx, t, "Cobalt Labs")
	lead := e.linkLead(ctx, t, e.owner, "kim@cobalt.test", "Cobalt Labs")
	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Fatalf("link: %v", err)
	}

	respelled, err := e.store.UpdateLead(ctx, lead, UpdateLeadInput{CompanyName: new("COBALT  labs")})
	if err != nil {
		t.Fatal(err)
	}
	if respelled.CompanyId == nil {
		t.Error("a name that still names the linked company dropped the link")
	}
	moved, err := e.store.UpdateLead(ctx, lead, UpdateLeadInput{CompanyName: new("Zinc Works")})
	if err != nil {
		t.Fatal(err)
	}
	if moved.CompanyId != nil {
		t.Errorf("company_id = %v after the name moved to another company, want none", *moved.CompanyId)
	}
}

func TestPromotingALinkedLeadCarriesTheEmployment(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.linkCompany(ctx, t, "Delta Mills")
	lead := e.linkLead(ctx, t, e.owner, "sam@delta.test", "Delta Mills")
	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Fatalf("link: %v", err)
	}

	promoted, err := e.store.QualifyLead(ctx, lead, PromoteLeadInput{Trigger: string(TriggerHumanQualify)})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if promoted.Merged {
		t.Fatal("the promotion merged; the link made a contact it should not have")
	}
	contact := ids.From[ids.ContactKind](ids.UUID(promoted.Contact.Id))
	if !e.employed(ctx, t, contact, company) {
		t.Error("the promoted contact holds no employment at the lead's linked company")
	}
}

// refusedAsNotTheMatch reports whether a link answered the 422 that names
// company_id as not the lead's current match.
func refusedAsNotTheMatch(err error) bool {
	var refusal *httperr.DetailedError
	return errors.As(err, &refusal) && len(refusal.Fields) == 1 &&
		refusal.Fields[0].Field == "company_id" && refusal.Fields[0].Code == "not_the_lead_match"
}

func TestALinkRefusesACompanyThatIsNotTheLeadsCurrentMatch(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)

	foxtrot := e.linkCompany(ctx, t, "Foxtrot Ltd")
	renamed := e.linkLead(ctx, t, e.owner, "a@foxtrot.test", "Foxtrot Ltd")
	if _, err := e.store.UpdateLead(ctx, renamed, UpdateLeadInput{CompanyName: new("Golf Ltd")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.LinkLeadCompany(ctx, renamed, foxtrot); !refusedAsNotTheMatch(err) {
		t.Errorf("a lead renamed away from the offered company linked it: %v", err)
	}

	hotel := e.linkCompany(ctx, t, "Hotel Group")
	e.linkCompany(ctx, t, "hotel  GROUP")
	twinned := e.linkLead(ctx, t, e.owner, "b@hotel.test", "Hotel Group")
	if _, err := e.store.LinkLeadCompany(ctx, twinned, hotel); !refusedAsNotTheMatch(err) {
		t.Errorf("a name two companies carry linked one of them: %v", err)
	}

	india := e.linkCompany(ctx, t, "India Co")
	e.linkCompany(ctx, t, "Juliet Co")
	other := e.linkLead(ctx, t, e.owner, "c@juliet.test", "Juliet Co")
	if _, err := e.store.LinkLeadCompany(ctx, other, india); !refusedAsNotTheMatch(err) {
		t.Errorf("a company the lead's name never named was linked: %v", err)
	}
	if got := e.contactsHolding(ctx, t, "c@juliet.test"); len(got) != 0 {
		t.Errorf("a refused link still made %d contact(s)", len(got))
	}
}

// leadTrail counts the lead's audit rows and lead.updated events, and reads its version.
func (e *privacyEnv) leadTrail(ctx context.Context, t *testing.T, lead ids.LeadID) (audits, events int, version int64) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM audit_log WHERE entity_type = 'lead' AND entity_id = $1),
			       (SELECT count(*) FROM event_outbox
			         WHERE envelope->'entity'->>'id' = $1::text AND envelope->>'type' = 'lead.updated'),
			       (SELECT version FROM lead WHERE id = $1)`, lead).Scan(&audits, &events, &version)
	}); err != nil {
		t.Fatalf("read the lead's trail: %v", err)
	}
	return audits, events, version
}

func TestRepeatingALinkWritesNothing(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.linkCompany(ctx, t, "Kilo Systems")
	lead := e.linkLead(ctx, t, e.owner, "k@kilo.test", "Kilo Systems")
	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Fatalf("link: %v", err)
	}
	audits, events, version := e.leadTrail(ctx, t, lead)

	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Fatalf("repeat the link: %v", err)
	}
	a, ev, v := e.leadTrail(ctx, t, lead)
	if a != audits || ev != events || v != version {
		t.Errorf("a repeated link wrote: audits %d -> %d, events %d -> %d, version %d -> %d",
			audits, a, events, ev, version, v)
	}
}

func TestMergingTheLinkedCompanyAwayMovesTheLead(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeAll)
	retired := e.linkCompany(ctx, t, "Lima Foods")
	survivor := e.linkCompany(ctx, t, "Lima Foods International")
	lead := e.linkLead(ctx, t, e.owner, "l@lima.test", "Lima Foods")
	if _, err := e.store.LinkLeadCompany(ctx, lead, retired); err != nil {
		t.Fatalf("link: %v", err)
	}
	audits, events, _ := e.leadTrail(ctx, t, lead)

	if _, err := e.store.MergeCompany(ctx, retired, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	var linked ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT company_id FROM lead WHERE id = $1`, lead).Scan(&linked)
	}); err != nil {
		t.Fatal(err)
	}
	if linked != survivor.UUID {
		t.Errorf("lead.company_id = %s after the merge, want the survivor %s", linked, survivor)
	}
	a, ev, _ := e.leadTrail(ctx, t, lead)
	if a != audits+1 || ev != events+1 {
		t.Errorf("the merge moved the lead with %d audit row(s) and %d lead.updated event(s), want 1 and 1",
			a-audits, ev-events)
	}
}

func TestTheLeadListWithholdsALinkedCompanyTheReaderCannotOpen(t *testing.T) {
	e := setupCapturePrivacy(t)
	ownerCtx := e.linker(e.owner, principal.RowScopeOwn)
	company := e.linkCompany(ownerCtx, t, "Quiet Holdings")
	lead := e.linkLead(ownerCtx, t, e.owner, "kim@quiet.test", "Quiet Holdings")
	if _, err := e.store.LinkLeadCompany(ownerCtx, lead, company); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := e.store.UpdateCompany(ownerCtx, company, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("make the company private: %v", err)
	}
	teammateCtx := e.linker(e.teammate, principal.RowScopeTeam)
	// The default order reads the work queue and a named sort reads the page;
	// both must withhold the company.
	for name, sort := range map[string]*string{"work queue": nil, "sorted page": new("full_name")} {
		t.Run(name, func(t *testing.T) {
			if got := listedCompanyID(ownerCtx, t, e.store, lead, sort); got == nil || ids.UUID(*got) != company.UUID {
				t.Fatalf("the owner's row: company_id = %v, want %s", got, company)
			}
			if got := listedCompanyID(teammateCtx, t, e.store, lead, sort); got != nil {
				t.Errorf("the teammate's row: company_id = %v, want it withheld", *got)
			}
		})
	}
}

// listedCompanyID lists leads as ctx's reader and returns the company id the
// row for lead carries.
func listedCompanyID(ctx context.Context, t *testing.T, store *Store, lead ids.LeadID, sort *string) *openapi_types.UUID {
	t.Helper()
	rows, _, err := store.ListLeads(ctx, ListLeadsInput{Sort: sort})
	if err != nil {
		t.Fatalf("list leads: %v", err)
	}
	for _, row := range rows {
		if ids.UUID(row.Id) == lead.UUID {
			return row.CompanyId
		}
	}
	t.Fatalf("lead %s is not on the reader's list", lead)
	return nil
}

func TestPromotingALeadWhoseCompanyMergedAwayEmploysAtTheSurvivor(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeAll)
	retired := e.linkCompany(ctx, t, "Mike Metals")
	survivor := e.linkCompany(ctx, t, "Mike Metals Group")
	lead := e.linkLead(ctx, t, e.owner, "m@mike.test", "Mike Metals")
	if _, err := e.store.LinkLeadCompany(ctx, lead, retired); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := e.store.MergeCompany(ctx, retired, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	promoted, err := e.store.QualifyLead(ctx, lead, PromoteLeadInput{Trigger: string(TriggerHumanQualify)})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(promoted.Contact.Id))
	if !e.employed(ctx, t, contact, survivor) {
		t.Error("the promoted contact holds no employment at the merge survivor")
	}
	if e.employed(ctx, t, contact, retired) {
		t.Error("the promoted contact was employed at the merged-away company")
	}
}

// mergeAudit reads the after side of the merge audit row on the surviving lead.
func (e *privacyEnv) mergeAudit(ctx context.Context, t *testing.T, survivor ids.LeadID) map[string]any {
	t.Helper()
	var after map[string]any
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT after FROM audit_log
			 WHERE entity_type = 'lead' AND entity_id = $1 AND action = 'merge'`, survivor).Scan(&after)
	}); err != nil {
		t.Fatalf("read the survivor's merge audit: %v", err)
	}
	return after
}

func TestALeadMergeCarriesTheLosersCompanyLinkFillOnly(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.linker(e.owner, principal.RowScopeOwn)
	november := e.linkCompany(ctx, t, "November Tools")
	oscar := e.linkCompany(ctx, t, "Oscar Paper")

	unlinked := e.linkLead(ctx, t, e.owner, "n1@november.test", "November Tools")
	loser := e.linkLead(ctx, t, e.owner, "n2@november.test", "November Tools")
	if _, err := e.store.LinkLeadCompany(ctx, loser, november); err != nil {
		t.Fatalf("link the loser: %v", err)
	}
	correlation := ids.NewV7()
	merged, err := e.store.MergeLead(principal.WithCorrelationID(ctx, correlation), loser, unlinked)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged.CompanyId == nil || ids.UUID(*merged.CompanyId) != november.UUID {
		t.Errorf("survivor company_id = %v, want the loser's %s", merged.CompanyId, november)
	}
	if got := e.mergeAudit(ctx, t, unlinked)["company_id"]; got != november.String() {
		t.Errorf("the survivor's merge audit records company_id %v, want %s", got, november)
	}
	if got := e.eventTypesOf(ctx, t, correlation); len(got) != 1 || got[0] != "lead.merged" {
		t.Errorf("the merge emitted %v, want one lead.merged", got)
	}

	keeper := e.linkLead(ctx, t, e.owner, "o1@oscar.test", "Oscar Paper")
	if _, err := e.store.LinkLeadCompany(ctx, keeper, oscar); err != nil {
		t.Fatalf("link the keeper: %v", err)
	}
	other := e.linkLead(ctx, t, e.owner, "o2@oscar.test", "November Tools")
	if _, err := e.store.LinkLeadCompany(ctx, other, november); err != nil {
		t.Fatalf("link the second loser: %v", err)
	}
	kept, err := e.store.MergeLead(ctx, other, keeper)
	if err != nil {
		t.Fatalf("merge into a linked survivor: %v", err)
	}
	if kept.CompanyId == nil || ids.UUID(*kept.CompanyId) != oscar.UUID {
		t.Errorf("a linked survivor's company_id = %v, want its own %s", kept.CompanyId, oscar)
	}
}
