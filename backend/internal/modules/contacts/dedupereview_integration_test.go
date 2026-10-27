// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The review lanes after filing. The nightly re-check keeps a pair while its
// evidence holds and retires it once that is gone. Every match is filed, and a
// promotion takes part. What a caller is shown depends only on records the
// caller can see.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// openPairID answers the id of the open contact or company pair joining a and
// b, or the zero id when there is none.
func (e *dedupeEnv) openPairID(ctx context.Context, t *testing.T, entityType string, a, b ids.UUID) ids.UUID {
	t.Helper()
	left, right := a, b
	if right.String() < left.String() {
		left, right = right, left
	}
	leftCol, rightCol := "left_contact_id", "right_contact_id"
	if entityType == entityCompany {
		leftCol, rightCol = "left_company_id", "right_company_id"
	}
	var id ids.UUID
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT coalesce((SELECT id FROM dedupe_candidate
			  WHERE entity_type = $1 AND disposition = 'open' AND `+leftCol+` = $2 AND `+rightCol+` = $3), $4)`,
			entityType, left, right, ids.Nil).Scan(&id)
	})
	if err != nil {
		t.Fatalf("read the pair of %s and %s: %v", a, b, err)
	}
	return id
}

// assertRecheckKeeps runs the scheduled re-check and asserts the pair is still
// open and not archived.
func (e *dedupeEnv) assertRecheckKeeps(ctx context.Context, t *testing.T, pair ids.UUID) {
	t.Helper()
	if pair == ids.Nil {
		t.Fatal("no pair was filed, so the re-check has nothing to keep")
	}
	e.recheck(t)
	if archived, _ := e.candidateState(ctx, t, pair); archived {
		t.Fatal("the re-check archived a pair whose evidence still holds")
	}
}

// assertRecheckRetires runs the re-check and asserts the pair is archived.
func (e *dedupeEnv) assertRecheckRetires(ctx context.Context, t *testing.T, pair ids.UUID) {
	t.Helper()
	e.recheck(t)
	if archived, _ := e.candidateState(ctx, t, pair); !archived {
		t.Fatal("the re-check kept a pair whose evidence is gone")
	}
}

func TestTheRecheckKeepsATaggedAddressPairUntilTheAddressMoves(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.plainContact(ctx, t, "Ab", "ab@example.com")
	created := e.plainContact(ctx, t, "Anna Beispiel", "ab+accounting@example.com")
	pair := e.openPairID(ctx, t, entityContact, incumbent, created)
	e.assertRecheckKeeps(ctx, t, pair)

	if _, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](created), UpdateContactInput{
		Emails: []ContactEmailInput{{Email: "anna@elsewhere.test", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("move the address: %v", err)
	}
	e.assertRecheckRetires(ctx, t, pair)
}

func TestTheRecheckKeepsADigraphNamePairUntilTheNameMoves(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.plainContact(ctx, t, "Björn Beispiel", "one@first.test")
	created := e.plainContact(ctx, t, "Bjoern Beispiel", "two@second.test")
	pair := e.openPairID(ctx, t, entityContact, incumbent, created)
	e.assertRecheckKeeps(ctx, t, pair)

	name := "Carla Andersson"
	if _, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](created), UpdateContactInput{FullName: &name}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	e.assertRecheckRetires(ctx, t, pair)
}

func TestTheRecheckKeepsASubdomainPairUntilTheDomainGoes(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	holder := e.createNamedCompany(ctx, t, "Example Group", "example.com")
	created := e.createNamedCompany(ctx, t, "Cloudy", "cloudy.test", "cloud.example.com")
	pair := e.openPairID(ctx, t, entityCompany, holder.UUID, created.UUID)
	e.assertRecheckKeeps(ctx, t, pair)

	desired := []CompanyDomainInput{{Domain: "cloudy.test", IsPrimary: true}}
	if _, err := e.store.UpdateCompany(ctx, created, UpdateCompanyInput{Domains: &desired}); err != nil {
		t.Fatalf("remove the subdomain: %v", err)
	}
	e.assertRecheckRetires(ctx, t, pair)
}

func TestADeeperSubdomainPairsWithAHeldIntermediateDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	holder := e.createNamedCompany(ctx, t, "Example Europe", "eu.example.com")
	created := e.createNamedCompany(ctx, t, "Team Site", "team.eu.example.com")
	pair := e.openPairID(ctx, t, entityCompany, holder.UUID, created.UUID)
	e.assertRecheckKeeps(ctx, t, pair)
	if n := e.countInWorkspace(ctx, t, `SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'company' AND confidence = 0`); n != 1 {
		t.Fatalf("%d unscored company pairs, want 1: a subdomain pair is evidence, not an exact identity", n)
	}
}

func TestEveryContactOnOneAddressBaseIsPaired(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := e.plainContact(ctx, t, "Ab", "ab@example.com")
	second := e.plainContact(ctx, t, "Anna Sales", "ab+sales@example.com")
	third := e.plainContact(ctx, t, "Anna Accounting", "ab+accounting@example.com")
	for _, pair := range [][2]ids.UUID{{first, second}, {first, third}, {second, third}} {
		if e.openPairID(ctx, t, entityContact, pair[0], pair[1]) == ids.Nil {
			t.Errorf("no pair between %s and %s", pair[0], pair[1])
		}
	}
}

func TestAContactOnTwoAddressBasesPairsWithBothHolders(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	one := e.plainContact(ctx, t, "Ab", "ab@example.com")
	other := e.plainContact(ctx, t, "Cd", "cd@other.test")
	created, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Anna Beispiel", Source: "manual",
		Emails: []ContactEmailInput{
			{Email: "ab+x@example.com", EmailType: "work", IsPrimary: true},
			{Email: "cd+y@other.test", EmailType: "work"},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, holder := range []ids.UUID{one, other} {
		if e.openPairID(ctx, t, entityContact, ids.UUID(created.Id), holder) == ids.Nil {
			t.Errorf("no pair with %s", holder)
		}
	}
}

// hideFromOther makes a contact the rep's private record, which the colleague
// cannot see.
func (e *dedupeEnv) hideFromOther(ctx context.Context, t *testing.T, id ids.UUID) {
	t.Helper()
	setRowVisibility(ctx, t, e, `UPDATE contact SET owner_id = $1, visibility = 'owner' WHERE id = $2`, &e.rep, id)
}

// colleagueOutcome is what the colleague is told about one proposed contact:
// the create preview's review target, and the vCard import's result.
type colleagueOutcome struct {
	previewErr    string
	previewTarget *ids.ContactID
	cardOutcome   string
	cardTarget    *ids.ContactID
}

func (e *dedupeEnv) otherPrincipal(ctx context.Context, t *testing.T) principal.Principal {
	t.Helper()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the colleague's context carries no actor")
	}
	return actor
}

func (e *dedupeEnv) colleagueSees(t *testing.T, name, email string) colleagueOutcome {
	t.Helper()
	ctx := e.asOther()
	var out colleagueOutcome
	preview, err := e.store.PreviewContactCreationForActor(ctx, CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	}, e.otherPrincipal(ctx, t))
	if err != nil {
		out.previewErr = err.Error()
	}
	out.previewTarget = preview.ReviewContactID
	results, err := e.store.ImportVCards(ctx, []VCardEntry{{FullName: name, Emails: []VCardChannel{{Value: email, Kind: "work"}}}})
	if err != nil || len(results) != 1 {
		t.Fatalf("import as the colleague: %+v, %v", results, err)
	}
	out.cardOutcome, out.cardTarget = string(results[0].Outcome), results[0].ContactID
	if results[0].Outcome == VCardCreated {
		out.cardTarget = nil
	}
	return out
}

// A record the caller cannot see changes nothing the caller is told, while the
// pair with it is still filed for whoever can see both.
func TestAHiddenAddressVariantChangesNothingTheCallerIsTold(t *testing.T) {
	without := setupDedupe(t)
	baseline := without.colleagueSees(t, "Zed Unrelated", "ab+x@example.com")

	with := setupDedupe(t)
	hidden := with.plainContact(with.as(), t, "Ab", "ab@example.com")
	with.hideFromOther(with.as(), t, hidden)
	got := with.colleagueSees(t, "Zed Unrelated", "ab+x@example.com")

	if got.previewErr != baseline.previewErr || (got.previewTarget == nil) != (baseline.previewTarget == nil) ||
		got.cardOutcome != baseline.cardOutcome || (got.cardTarget == nil) != (baseline.cardTarget == nil) {
		t.Fatalf("with a hidden record the colleague is told %+v, without it %+v", got, baseline)
	}
	if n := with.countInWorkspace(with.as(), t, `
		SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'contact' AND $1 IN (left_contact_id, right_contact_id)`,
		hidden); n != 1 {
		t.Fatalf("%d pairs with the hidden record, want the one the import's create filed", n)
	}
}

// A hidden holder ranked first does not mask a visible one.
func TestAHiddenFirstVariantDoesNotMaskAVisibleOne(t *testing.T) {
	e := setupDedupe(t)
	hidden := e.plainContact(e.as(), t, "Ab", "ab@example.com")
	e.hideFromOther(e.as(), t, hidden)
	visible := e.plainContact(e.as(), t, "Anna Sales", "ab+sales@example.com")

	got := e.colleagueSees(t, "Zed Unrelated", "ab+x@example.com")
	if got.previewErr != "" || got.previewTarget == nil || got.previewTarget.UUID != visible {
		t.Fatalf("preview %+v, want the visible holder %s", got, visible)
	}
	if got.cardOutcome != string(VCardNeedsReview) || got.cardTarget == nil || got.cardTarget.UUID != visible {
		t.Fatalf("import %+v, want a possible duplicate of the visible holder %s", got, visible)
	}
}

func TestPromotingALeadOnATaggedAddressCreatesAndPairs(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := e.plainContact(ctx, t, "Ab", "ab@example.com")
	lead := seedHistoricalLeadWithEmail(ctx, t, e.store, CreateLeadInput{
		FullName: new("Anna Beispiel"), Source: "manual",
	}, "ab+x@example.com")

	change, err := e.store.PreviewLeadContactChange(ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || change.ReviewContact == nil || change.ReviewContact.UUID != incumbent {
		t.Fatalf("preview %+v (err %v), want the review target %s", change, err, incumbent)
	}
	contact, merged, err := e.store.PromoteLead(ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || merged || ids.UUID(contact.Id) == incumbent {
		t.Fatalf("promote = %s merged %v (err %v), want a new contact beside %s", contact.Id, merged, err, incumbent)
	}
	if e.openPairID(ctx, t, entityContact, ids.UUID(contact.Id), incumbent) == ids.Nil {
		t.Fatal("the promotion filed no pair with the base address's contact")
	}
}

func TestPromotingALeadOnAnExactAddressStillAdoptsOnlyThatContact(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.plainContact(ctx, t, "Ab", "ab@example.com")
	exact := e.plainContact(ctx, t, "Anna Beispiel", "ab+x@example.com")
	lead := seedHistoricalLeadWithEmail(ctx, t, e.store, CreateLeadInput{
		FullName: new("Anna Beispiel"), Source: "manual",
	}, "ab+x@example.com")

	contact, merged, err := e.store.PromoteLead(ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || !merged || ids.UUID(contact.Id) != exact {
		t.Fatalf("promote = %s merged %v (err %v), want the exact holder %s", contact.Id, merged, err, exact)
	}
	if n := e.countInWorkspace(ctx, t, `SELECT count(*) FROM contact WHERE archived_at IS NULL`); n != 2 {
		t.Fatalf("%d live contacts, want 2: an exact promotion creates nothing", n)
	}
}
