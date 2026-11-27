// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The rule for combining two contacts' values (mergeprecedence.go). Every value
// stays once. Each single slot goes to the preferred member, else the survivor,
// else the merged-away contact.

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sharedNumber = "+493055500001"

// contactWithPhones creates a contact through the real create, primary first.
func contactWithPhones(t *testing.T, e *dedupeEnv, name, email string, phones ...string) ids.ContactID {
	t.Helper()
	in := CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	}
	for i, phone := range phones {
		in.Phones = append(in.Phones, ContactPhoneInput{Phone: phone, PhoneType: "mobile", IsPrimary: i == 0, Position: i})
	}
	c, err := e.store.CreateContact(e.as(), in)
	if err != nil {
		t.Fatalf("create contact %q: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(c.Id))
}

func survivorOf(ctx context.Context, t *testing.T, e *dedupeEnv, id ids.ContactID) crmcontracts.Contact {
	t.Helper()
	c, err := e.store.GetContact(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read the survivor: %v", err)
	}
	return c
}

// survivorPhones lists the survivor's live numbers, and the primary one.
func survivorPhones(c crmcontracts.Contact) (numbers []string, primary string) {
	if c.Phones == nil {
		return nil, ""
	}
	for _, p := range *c.Phones {
		if p.ArchivedAt != nil {
			continue
		}
		numbers = append(numbers, p.Phone)
		if p.IsPrimary {
			primary = p.Phone
		}
	}
	slices.Sort(numbers)
	return numbers, primary
}

func TestMergingThreeContactsThatShareANumberKeepsItOnce(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.editorCtx()
	survivor := contactWithPhones(t, e, "Alice Wong", "alice@example.com", sharedNumber)
	second := contactWithPhones(t, e, "Alice W.", "alice.w@example.com", sharedNumber, "+493055500002")
	third := contactWithPhones(t, e, "A. Wong", "a.wong@example.com", sharedNumber)

	for _, source := range []ids.ContactID{second, third} {
		if _, err := e.store.MergeContact(ctx, source, survivor, nil); err != nil {
			t.Fatalf("merge: %v", err)
		}
	}

	numbers, primary := survivorPhones(survivorOf(ctx, t, e, survivor))
	if want := []string{sharedNumber, "+493055500002"}; !slices.Equal(numbers, want) {
		t.Errorf("the survivor's numbers = %v, want each number once: %v", numbers, want)
	}
	if primary != sharedNumber {
		t.Errorf("the primary number = %q, want the survivor's own %q", primary, sharedNumber)
	}
	var counted int64
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT (after->>'relinked_phones')::bigint FROM audit_log
 WHERE entity_id = $1 AND action = 'merge' AND after->>'merged_from_id' = $2::text`, survivor, second).Scan(&counted)
	}); err != nil {
		t.Fatal(err)
	}
	if counted != 2 {
		t.Errorf("the merge counted %d relinked numbers, want 2: the folded one counts too", counted)
	}
	var moved int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM contact_phone WHERE contact_id = ANY($1)`,
			[]ids.ContactID{second, third}).Scan(&moved)
	}); err != nil {
		t.Fatal(err)
	}
	if moved != 0 {
		t.Errorf("%d numbers stayed on the merged-away contacts, want every row moved to the survivor", moved)
	}
}

func TestThePreferredMemberWinsTheSingleSlots(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Bea Lund", "Old Employer", "Preferred Employer")
	survivor := contactWithPhones(t, e, "Beatrice Lund", "bea@old.example")
	e.employAt(ctx, t, survivor, fx.first, true)
	e.employAt(ctx, t, fx.contact, fx.second, true)
	if _, err := e.store.UpdateContact(ctx, fx.contact, UpdateContactInput{Title: new("Head of Sales")}); err != nil {
		t.Fatalf("title the source: %v", err)
	}
	if _, err := e.store.UpdateContact(ctx, survivor, UpdateContactInput{Title: new("Sales Rep")}); err != nil {
		t.Fatalf("title the survivor: %v", err)
	}
	addEmail(ctx, t, e, fx.contact, "bea@preferred.example")
	prefer := MergePreference{}
	for _, field := range mergeFields {
		prefer[field] = fx.contact.UUID
	}

	mergeWithPreference(ctx, t, e, fx.contact, survivor, prefer)

	got := survivorOf(ctx, t, e, survivor)
	if got.PrimaryEmail == nil || string(*got.PrimaryEmail) != "bea@preferred.example" {
		t.Errorf("primary email = %s, want the preferred member's", valueOf(got.PrimaryEmail))
	}
	if got.FullName != "Bea Lund" {
		t.Errorf("name = %q, want the preferred member's", got.FullName)
	}
	if got.Emails == nil || len(*got.Emails) != 2 {
		t.Errorf("emails = %v, want both addresses kept", got.Emails)
	}
	if got.Title == nil || *got.Title != "Head of Sales" {
		t.Errorf("title = %s, want the preferred member's", valueOf(got.Title))
	}
	if holder := slotHolder(ctx, t, e, survivor); holder.IsZero() || companyOfEdge(ctx, t, e, holder) != fx.second.UUID {
		t.Errorf("the current employer is not the preferred member's")
	}
}

func TestWithoutAPreferenceTheSurvivorKeepsItsSlots(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Cai Moreno", "Own Employer", "Other Employer")
	survivor := contactWithPhones(t, e, "Cai Moreno", "cai@own.example")
	own := e.employAt(ctx, t, survivor, fx.first, true)
	e.employAt(ctx, t, fx.contact, fx.second, true)
	addEmail(ctx, t, e, fx.contact, "cai@other.example")

	mergeWithPreference(ctx, t, e, fx.contact, survivor, nil)

	got := survivorOf(ctx, t, e, survivor)
	if got.PrimaryEmail == nil || string(*got.PrimaryEmail) != "cai@own.example" {
		t.Errorf("primary email = %s, want the survivor's own", valueOf(got.PrimaryEmail))
	}
	if holder := slotHolder(ctx, t, e, survivor); holder != own.ID {
		t.Errorf("the current employer moved to %v, want the survivor's own %v", holder, own.ID)
	}
}

func addEmail(ctx context.Context, t *testing.T, e *dedupeEnv, contact ids.ContactID, email string) {
	t.Helper()
	if _, err := e.store.UpdateContact(ctx, contact, UpdateContactInput{
		Emails: []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("give the contact %s: %v", email, err)
	}
}

func mergeWithPreference(ctx context.Context, t *testing.T, e *dedupeEnv, source, target ids.ContactID, prefer MergePreference) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, _, err := e.store.mergeContactCarryingTx(ctx, tx, source, target, nil, nil, prefer)
		return err
	}); err != nil {
		t.Fatalf("merge: %v", err)
	}
}

func companyOfEdge(ctx context.Context, t *testing.T, e *dedupeEnv, edge ids.UUID) ids.UUID {
	t.Helper()
	var company ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT company_id FROM relationship WHERE id = $1`, edge).Scan(&company)
	}); err != nil {
		t.Fatal(err)
	}
	return company
}

// Enrichment applies rule 1 too: a signature restating a number the contact
// already holds confirms it and adds no second row.
func TestASignatureRestatingAHeldNumberAddsNoRow(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	contact := contactWithPhones(t, e, "Dina Roth", "dina@example.com", sharedNumber)

	fillFromSignature(ctx, t, e, contact, SignatureField{
		Name: fieldPhone, Value: "+49 30 55500001", Evidence: "+49 30 55500001", Confidence: 0.9,
	})

	if got := livePhones(ctx, t, e, contact); !slices.Equal(got, []string{sharedNumber}) {
		t.Errorf("live numbers = %v, want the held number once", got)
	}
}

func valueOf[T ~string](p *T) string {
	if p == nil {
		return "<nil>"
	}
	return string(*p)
}

// A preferred employer the survivor once held and left is a separate edge, not a
// twin. The member's current edge takes the slot, so the survivor keeps a
// current employer.
func TestAPreferredEmployerTheSurvivorLeftBecomesItsCurrentOne(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.editorCtx()
	fx := e.seedFlipFixture(ctx, t, "Eda Varga", "Current Employer", "Former Employer")
	survivor := contactWithPhones(t, e, "Eda Varga", "eda@current.example")
	e.employAt(ctx, t, survivor, fx.first, true)
	ended, notPrimary := clocktest.Now(t).AddDate(-1, 0, 0), false
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: employmentKind, ContactID: &survivor, CompanyID: &fx.second, IsCurrentPrimary: &notPrimary, EndedAt: &ended, Source: "manual",
	}); err != nil {
		t.Fatalf("the survivor's former employment: %v", err)
	}
	e.employAt(ctx, t, fx.contact, fx.second, true)

	mergeWithPreference(ctx, t, e, fx.contact, survivor, MergePreference{MergeFieldEmployment: fx.contact.UUID})

	holder := slotHolder(ctx, t, e, survivor)
	var current bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT `+employment.IsCurrentSQL("ended_at")+` FROM relationship WHERE id = $1`, holder).Scan(&current)
	}); err != nil || !current || companyOfEdge(ctx, t, e, holder) != fx.second.UUID {
		t.Errorf("the slot is held by %v (current=%t), want the member's current edge at the preferred employer: %v", holder, current, err)
	}
}

// The address the review shows for a member is its reachable-first one, so a
// member with no primary address still wins with that address.
func TestAPreferredMemberWithNoPrimaryAddressWinsWithItsFirstAddress(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.editorCtx()
	survivor := contactWithPhones(t, e, "Fia Holm", "fia@own.example")
	member := contactWithPhones(t, e, "Fia H.", "fia@member.example")
	if _, err := e.store.UpdateContact(ctx, member, UpdateContactInput{
		Emails: []ContactEmailInput{{Email: "fia@member.example", EmailType: "work"}},
	}); err != nil {
		t.Fatalf("clear the member's primary address: %v", err)
	}

	mergeWithPreference(ctx, t, e, member, survivor, MergePreference{MergeFieldEmail: member.UUID})

	if got := survivorOf(ctx, t, e, survivor); got.PrimaryEmail == nil || string(*got.PrimaryEmail) != "fia@member.example" {
		t.Errorf("primary email = %s, want the member's address the review showed", valueOf(got.PrimaryEmail))
	}
}
