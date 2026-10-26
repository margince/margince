// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A lead's company link over a real Postgres. These cover the offer a read
// makes, and the confirm that writes the link, the contact and the employment.
// They also cover what an edit of the company name and a promotion do to it.

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

// linker binds one user with every grant a link needs, at one row scope.
func (e *privacyEnv) linker(user ids.UUID, scope principal.RowScope) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		TeamIDs: []ids.UUID{e.team},
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"contact":      {Create: true, Read: true, Update: true},
				"company":      {Create: true, Read: true, Update: true},
				"lead":         {Create: true, Read: true, Update: true},
				"relationship": {Create: true, Read: true, Update: true},
			},
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

func TestLinkingALeadWritesTheCompanyTheContactAndTheEmployment(t *testing.T) {
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

	linked, err := e.store.LinkLeadCompany(ctx, lead, company)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if linked.CompanyId == nil || ids.UUID(*linked.CompanyId) != company.UUID {
		t.Errorf("company_id = %v, want %s", linked.CompanyId, company)
	}
	if linked.Status != offered.Status || linked.ArchivedAt != nil || linked.PromotedContactId != nil {
		t.Errorf("the link promoted the lead: status %s -> %s, archived %v", offered.Status, linked.Status, linked.ArchivedAt)
	}
	contacts := e.contactsHolding(ctx, t, "lena@acme-rocket.test")
	if len(contacts) != 1 {
		t.Fatalf("contacts holding the lead's address = %d, want 1", len(contacts))
	}
	if !e.employed(ctx, t, contacts[0], company) {
		t.Error("the new contact holds no employment at the linked company")
	}
	if n := e.linkAudits(ctx, t, lead); n != 1 {
		t.Errorf("audit rows recording the link = %d, want 1", n)
	}
	again, err := e.store.GetLead(ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if again.CompanyMatch != nil {
		t.Error("a linked lead still offers a company match")
	}
}

func TestLinkingALeadLandsOnTheContactItsEmailAlreadyNames(t *testing.T) {
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
	lead := e.linkLead(ctx, t, e.owner, "lena@boreal.test", "Boreal Freight")

	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Fatalf("link: %v", err)
	}
	contacts := e.contactsHolding(ctx, t, "lena@boreal.test")
	if len(contacts) != 1 || contacts[0].UUID != ids.UUID(existing.Id) {
		t.Fatalf("contacts holding the address = %v, want only the existing %s", contacts, existing.Id)
	}
	if !e.employed(ctx, t, contacts[0], company) {
		t.Error("the existing contact holds no employment at the linked company")
	}
	// A second confirm keeps the one employment rather than refusing on it.
	if _, err := e.store.LinkLeadCompany(ctx, lead, company); err != nil {
		t.Errorf("linking again: %v", err)
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
	// A new address matches no contact, so the promotion creates one, and only
	// the carry can give that contact its employment.
	if _, err := e.store.UpdateLead(ctx, lead, UpdateLeadInput{Email: new("sam.new@delta.test")}); err != nil {
		t.Fatal(err)
	}

	promoted, err := e.store.QualifyLead(ctx, lead, PromoteLeadInput{Trigger: string(TriggerHumanQualify)})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if promoted.Merged {
		t.Fatal("the promotion merged; the fixture meant it to create a contact")
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
