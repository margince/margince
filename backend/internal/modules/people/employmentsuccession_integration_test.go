// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Who answers "where do they work now" once the job that answered it is over.
//
// The flag is kept true at the three moments a caller states something. This is
// the moment nobody states anything — the primary edge is retired — and the two
// doors into it are archiving that edge and ending it by patch. Both are here,
// because covering one leaves the same person reachable through the other.
//
// The three shapes the ruling names, on both doors: one employment left
// (promoted, because it is the only answer there is), two left (unset, because
// choosing between two employers on somebody's behalf writes a fact nobody
// asserted), and none left (unset, and nothing invented).

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// employAlso adds a second employment for a person, without the flag: the
// incumbent holds the slot, and this is the row a departure might promote.
func (e *dedupeEnv) employAlso(ctx context.Context, t *testing.T, person ids.PersonID, companyName, domain string) {
	t.Helper()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{
		DisplayName: companyName, Source: "manual",
		Domains: []CompanyDomainInput{{Domain: domain, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding %s: %v", companyName, err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: "employment", PersonID: &person, CompanyID: &companyID, Source: "manual",
	}); err != nil {
		t.Fatalf("seeding the second employment at %s: %v", companyName, err)
	}
}

// employmentEdge answers the id of this person's employment at the company the
// seed gave them, which is the edge the departures below retire.
func (e *dedupeEnv) employmentEdge(ctx context.Context, t *testing.T, person ids.PersonID, company ids.CompanyID) ids.UUID {
	t.Helper()
	var edge ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM relationship
			  WHERE kind = 'employment' AND person_id = $1 AND company_id = $2`,
			person, company).Scan(&edge)
	}); err != nil {
		t.Fatalf("reading the person's employment edge: %v", err)
	}
	return edge
}

// primaryEmployer answers which company holds this person's current-primary
// flag, or the nil UUID for none — through the production predicates, so a
// definition that moves underneath cannot leave this passing.
func (e *dedupeEnv) primaryEmployer(ctx context.Context, t *testing.T, person ids.PersonID) ids.UUID {
	t.Helper()
	var company *ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT (SELECT company_id FROM relationship
			         WHERE person_id = $1 AND `+employment.CurrentPrimarySlotSQL("")+`
			           AND `+employment.IsCurrentSQL("ended_at")+`)`, person).Scan(&company)
	}); err != nil {
		t.Fatalf("reading the person's primary employer: %v", err)
	}
	if company == nil {
		return ids.Nil
	}
	return *company
}

// asRetirer is the editor with the one grant archiving needs, added to the
// fixture's own principal rather than spelled as a second permission map: a
// second copy would be a second answer to "what may an editor do", and this
// case is about one verb more than the editor already has.
func (e *dedupeEnv) asRetirer(t *testing.T) context.Context {
	t.Helper()
	ctx := e.asEditor()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the editor context carries no actor")
	}
	grants := map[string]principal.ObjectGrant{}
	for object, grant := range actor.Permissions.Objects {
		grants[object] = grant
	}
	relationship := grants["relationship"]
	relationship.Delete = true
	grants["relationship"] = relationship
	actor.Permissions.Objects = grants
	return principal.WithActor(ctx, actor)
}

// retire is one of the two doors, named so a case can run both without saying
// which it is on.
type retire struct {
	door string
	run  func(ctx context.Context, t *testing.T, e *dedupeEnv, edge ids.UUID)
}

func bothDoors() []retire {
	return []retire{
		{door: "archived", run: func(ctx context.Context, t *testing.T, e *dedupeEnv, edge ids.UUID) {
			t.Helper()
			if _, err := e.store.ArchiveRelationship(e.asRetirer(t), edge, nil); err != nil {
				t.Fatalf("archiving the primary employment: %v", err)
			}
		}},
		{door: "ended by patch", run: func(ctx context.Context, t *testing.T, e *dedupeEnv, edge ids.UUID) {
			t.Helper()
			// Yesterday, not today: employment.IsCurrentSQL reads a date that
			// has arrived as a departure, and a future one as notice served.
			ended := time.Now().UTC().AddDate(0, 0, -1)
			if _, err := e.store.UpdateRelationship(e.asEditor(), edge,
				UpdateRelationshipInput{EndedAt: &ended}); err != nil {
				t.Fatalf("ending the primary employment: %v", err)
			}
		}},
	}
}

// One remaining employment is not a choice — it is the only answer, so the
// product says it rather than leaving a person plainly employed with no
// employer on their page.
func TestRetiringThePrimaryEmploymentPromotesTheOneThatRemains(t *testing.T) {
	for _, door := range bothDoors() {
		t.Run(door.door, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			person, incumbentCompany := e.seedEmployedPerson(ctx, t,
				"Successor Subject", "successor@leaving.test", "Leaving GmbH", "leaving.test")
			e.employAlso(ctx, t, person, "Staying GmbH", "staying-"+door.door[:3]+".test")

			door.run(ctx, t, e, e.employmentEdge(ctx, t, person, incumbentCompany))

			got := e.primaryEmployer(ctx, t, person)
			if got == ids.Nil {
				t.Fatal("the person has one employment left and no primary employer — the answer was not a choice, and the page reads as though they work nowhere")
			}
			if got == incumbentCompany.UUID {
				t.Fatalf("the retired employer %s still holds the flag", got)
			}
		})
	}
}

// Two remaining IS a choice, and it is not the product's to make: picking
// between two employers on somebody's behalf writes a fact nobody asserted.
func TestRetiringThePrimaryEmploymentLeavesTwoSurvivorsUnchosen(t *testing.T) {
	for _, door := range bothDoors() {
		t.Run(door.door, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			person, incumbentCompany := e.seedEmployedPerson(ctx, t,
				"Twice Employed", "twice@leaving.test", "Leaving GmbH", "leaving.test")
			e.employAlso(ctx, t, person, "First Survivor GmbH", "first-"+door.door[:3]+".test")
			e.employAlso(ctx, t, person, "Second Survivor GmbH", "second-"+door.door[:3]+".test")

			door.run(ctx, t, e, e.employmentEdge(ctx, t, person, incumbentCompany))

			if got := e.primaryEmployer(ctx, t, person); got != ids.Nil {
				t.Errorf("the product chose %s between two remaining employers — is_current_primary is a column humans set deliberately, and this is where that means something", got)
			}
		})
	}
}

// None remaining invents nothing: a person who has left their only job has no
// current employer, and the empty slot is the honest answer.
func TestRetiringTheOnlyEmploymentLeavesNoEmployer(t *testing.T) {
	for _, door := range bothDoors() {
		t.Run(door.door, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			person, incumbentCompany := e.seedEmployedPerson(ctx, t,
				"Last Job", "lastjob@leaving.test", "Leaving GmbH", "leaving.test")

			door.run(ctx, t, e, e.employmentEdge(ctx, t, person, incumbentCompany))

			if got := e.primaryEmployer(ctx, t, person); got != ids.Nil {
				t.Errorf("a person with no employment left reads as employed at %s", got)
			}
		})
	}
}

// A human saying "this is not their primary employer" is a STATEMENT, and the
// promotion may not answer it by putting the flag straight back. It is the one
// case where the empty slot is an instruction rather than a gap — and the case
// that separates "an employment was retired" from "any employment changed".
func TestClearingTheFlagOnTheOnlyEmploymentIsNotOverruled(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	person, company := e.seedEmployedPerson(ctx, t,
		"Deliberate", "deliberate@leaving.test", "Leaving GmbH", "leaving.test")

	no := false
	if _, err := e.store.UpdateRelationship(e.asEditor(), e.employmentEdge(ctx, t, person, company),
		UpdateRelationshipInput{IsCurrentPrimary: &no}); err != nil {
		t.Fatalf("clearing the flag: %v", err)
	}

	if got := e.primaryEmployer(ctx, t, person); got != ids.Nil {
		t.Errorf("the flag came back on %s — a caller stating the answer was overruled with the answer they had just rejected", got)
	}
}
