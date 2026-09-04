// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The employer every person read carries: the contact list's company column and
// the record read answer it from one attach, so these drive both.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// employPerson records an employment edge through the writer production uses,
// ended when a date is given. A hand-inserted row would prove nothing about the
// rows the product makes — the current-primary flag among them.
func employPerson(t *testing.T, e *Env, person, org ids.UUID, ended *time.Time) {
	t.Helper()
	personID := ids.From[ids.PersonKind](person)
	orgID := ids.From[ids.OrganizationKind](org)
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind:             "employment",
		PersonID:         &personID,
		OrganizationID:   &orgID,
		IsCurrentPrimary: boolPtr(ended == nil),
		EndedAt:          ended,
		Source:           "manual",
	}); err != nil {
		t.Fatalf("seeding the employment edge: %v", err)
	}
}

// listedPerson is one row of the person list, by id.
func listedPerson(ctx context.Context, t *testing.T, e *Env, person ids.UUID) crmcontracts.Person {
	t.Helper()
	page, _, err := e.People.ListPeople(ctx, people.ListPeopleInput{})
	if err != nil {
		t.Fatalf("listing people: %v", err)
	}
	for _, row := range page {
		if ids.UUID(row.Id) == person {
			return row
		}
	}
	t.Fatalf("the person list returned %d rows, none of them the person under test", len(page))
	return crmcontracts.Person{}
}

func TestAContactNamesTheEmployerTheyHoldToday(t *testing.T) {
	e := Setup(t)
	acme := e.SeedOrg(t, "Acme", nil)
	former := e.SeedOrg(t, "Former Employer", nil)
	person := e.SeedPerson(t, "Anna Weber", nil)
	left := time.Date(2021, 6, 30, 0, 0, 0, 0, time.UTC)
	employPerson(t, e, person, former, &left)
	employPerson(t, e, person, acme, nil)

	// Both reads, because they share one attach and a reader asking "who is
	// this and where do they work" asks the same question on either surface.
	row := listedPerson(e.Admin(), t, e, person)
	record, err := e.People.GetPerson(e.Admin(), ids.From[ids.PersonKind](person), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the person: %v", err)
	}
	for name, got := range map[string]*crmcontracts.PersonEmployer{
		"the list row": row.Employer,
		"the record":   record.Employer,
	} {
		if got == nil {
			t.Fatalf("%s carries no employer, want Acme", name)
		}
		// The account they work at today. A read that answered with the job
		// they left names a company this contact cannot be reached at.
		if ids.UUID(got.OrganizationId) != acme || got.OrganizationName != "Acme" {
			t.Errorf("%s names %s (%v), want Acme (%v)", name, got.OrganizationName, got.OrganizationId, acme)
		}
	}
}

func TestAContactWhoseOnlyEmploymentEndedNamesNoEmployer(t *testing.T) {
	e := Setup(t)
	former := e.SeedOrg(t, "Former Employer", nil)
	person := e.SeedPerson(t, "Left Last Year", nil)
	left := time.Date(2021, 6, 30, 0, 0, 0, 0, time.UTC)
	employPerson(t, e, person, former, &left)

	if got := listedPerson(e.Admin(), t, e, person).Employer; got != nil {
		t.Errorf("a leaver still names %s as their employer", got.OrganizationName)
	}
}

// The employer is two disclosures, and losing either one loses the field and
// keeps the contact. Who works where is a fact about the PAIR, which the grant
// on the person does not cover; the name is the account's own to disclose. A
// caller short of either still gets their contacts — the contract says an
// absent employer never means "works nowhere", which is what stops the omission
// being read as an answer.
func TestTheEmployerNeedsBothTheEdgeAndTheCompanyGrant(t *testing.T) {
	e := Setup(t)
	acme := e.SeedOrg(t, "Acme", nil)
	person := e.SeedPerson(t, "Anna Weber", nil)
	employPerson(t, e, person, acme, nil)

	for missing, grants := range map[string]map[string]principal.ObjectGrant{
		"relationship": {objPerson: {Read: true}, objOrg: {Read: true}},
		"organization": {objPerson: {Read: true}, objRelationship: {Read: true}},
	} {
		partial := e.As(e.AdminUser, nil, principal.Permissions{
			RoleKeys: []string{roleReadOnly},
			Objects:  grants,
			RowScope: principal.RowScopeAll,
		})
		if got := listedPerson(partial, t, e, person).Employer; got != nil {
			t.Errorf("without the %s grant the row still names %s", missing, got.OrganizationName)
		}
	}
}
