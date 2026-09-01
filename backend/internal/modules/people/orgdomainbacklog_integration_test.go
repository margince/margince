// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The sweep attaches the people already on a company's domain.
//
// They accumulate while nobody has a company for the domain: capture creates
// each person and deliberately leaves the employer undecided, so by the time the
// company exists its whole roster is sitting there attached to nothing. The
// domain-triage verdict wired that backlog for the domain it judged; a company a
// human types in reached nobody.
//
// It is a SWEEP and not a write on the create, deliberately. Attaching a person
// to a company is a write about the PERSON, and the human naming a company holds
// no authority over contacts they may not see — a rep scoped to their own
// records would otherwise plant employment for a colleague's private contact as
// a side effect of typing in a company name.
//
// What it cost is visible on the account page rather than in an error: the
// company shows one contact — whichever sender writes next and gets an edge from
// their own ensure — and the health card says one person carries the whole
// relationship about a company with forty.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// employerOf answers which organization a person's live employment edge names.
func (e *dedupeEnv) employerOf(ctx context.Context, t *testing.T, person ids.PersonID) *ids.UUID {
	t.Helper()
	var org *ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT organization_id FROM relationship
			 WHERE person_id = $1 AND kind = 'employment'
			   AND `+EmploymentIsCurrentSQL("ended_at")+`
			   AND archived_at IS NULL
			 LIMIT 1`, person).Scan(&org)
	}); err != nil {
		if err.Error() == "no rows in result set" {
			return nil
		}
		t.Fatal(err)
	}
	return org
}

func TestTheSweepAttachesACompanysPeopleThatNoWriteReaches(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	// Two people arrive by capture while the domain has no company. Each ensure
	// creates the person and opens the organization question rather than
	// inventing a company from the domain label.
	first, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "ann@backlog.test", "Ann Backlog", "backlog.test"))
	if err != nil {
		t.Fatalf("ensure first: %v", err)
	}
	second, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "bo@backlog.test", "Bo Backlog", "backlog.test"))
	if err != nil {
		t.Fatalf("ensure second: %v", err)
	}
	if e.employerOf(ctx, t, first.PersonID) != nil {
		t.Fatal("a person was attached to a company before one existed; the test proves nothing")
	}

	// A human types the company in, naming the domain. The create itself plants
	// nothing — it has no authority to.
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Backlog GmbH",
		Domains:     []OrgDomainInput{{Domain: "backlog.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}

	// The sweep sees the company owed its people, and attaches them.
	owed, err := e.store.DomainsOwedTheirPeople(ctx, 50)
	if err != nil {
		t.Fatalf("listing the domains owed their people: %v", err)
	}
	var found bool
	for _, d := range owed {
		if d.OrganizationID.UUID == ids.UUID(org.Id) && d.Domain == "backlog.test" {
			found = true
			if _, err := e.store.AttachDomainBacklog(ctx, d); err != nil {
				t.Fatalf("attaching the backlog: %v", err)
			}
		}
	}
	if !found {
		t.Fatalf("the sweep does not offer %s, whose domain has contacts attached to nothing", org.Id)
	}

	for _, person := range []struct {
		name string
		id   ids.PersonID
	}{{"first", first.PersonID}, {"second", second.PersonID}} {
		employer := e.employerOf(ctx, t, person.id)
		if employer == nil {
			t.Errorf("%s contact has no employer after the sweep ran — "+
				"the account shows one contact and the health card blames the relationship on one person", person.name)
			continue
		}
		if *employer != ids.UUID(org.Id) {
			t.Errorf("%s contact works at %s, want the company just created %s", person.name, *employer, org.Id)
		}
	}

	// And the sweep drains: a company whose people are attached is not offered
	// again, or every nightly tick rewrites the same rows.
	after, err := e.store.DomainsOwedTheirPeople(ctx, 50)
	if err != nil {
		t.Fatalf("listing again: %v", err)
	}
	for _, d := range after {
		if d.OrganizationID.UUID == ids.UUID(org.Id) {
			t.Errorf("the sweep still offers %s after attaching its people; the selection does not drain", org.Id)
		}
	}
}

// A person the plant will REFUSE must not be offered, or the domain is returned
// on every tick for ever.
//
// uq_rel_employment admits one live employment per (person, organization), so
// somebody already holding a non-primary edge to this company has no
// current-primary slot — which is what the selector asks about — and is still a
// row the insert silently drops.
func TestTheSweepSkipsADomainWhosePeopleThePlantWillRefuse(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	person, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "held@refuse.test", "Already Held", "refuse.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Refuse GmbH",
		Domains:     []OrgDomainInput{{Domain: "refuse.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}

	// A live employment to this same company that does NOT hold the primary
	// slot: the index refuses a second, and the slot test alone would not see it.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO relationship (kind, person_id, organization_id, is_current_primary, source, captured_by)
			VALUES ('employment', $1, $2, false, 'manual', 'human:test')`,
			person.PersonID, ids.UUID(org.Id))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	owed, err := e.store.DomainsOwedTheirPeople(ctx, 50)
	if err != nil {
		t.Fatalf("listing the domains owed their people: %v", err)
	}
	for _, d := range owed {
		if d.OrganizationID.UUID == ids.UUID(org.Id) {
			t.Error("the sweep offers a domain whose only unattached person the plant will refuse; " +
				"the insert writes nothing and the domain comes back on every tick")
		}
	}
}
