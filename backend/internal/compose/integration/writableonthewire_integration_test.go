// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The `writable` flag a record carries must mean what it says.
//
// A client draws its edit affordances from it, so a flag that is merely PRESENT
// is worse than none: it would put a save button on a row the server then
// refuses, which is the guess this field exists to replace. The test is
// therefore not "the flag is set" but "the flag agrees with the mutation" —
// writable is true exactly when the write is admitted.
//
// It is UX honesty and never enforcement. The server's own gate is asserted
// separately, in writeauthority_integration_test.go and the agent parity suite;
// nothing here would change if a client ignored the flag entirely.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestARecordSaysWhetherItIsThisCallersToChange(t *testing.T) {
	e := Setup(t)
	svc := identity.NewService(e.Pool)
	pipeline, open, _ := DealFixture(t, e)
	title := "Changed by the writable suite"

	mine := e.SeedPerson(t, "My contact", &e.Rep1)
	theirs := e.SeedPerson(t, "Their contact", &e.Rep3)
	shared := e.SeedPerson(t, "Shared contact", &e.Rep3)
	if _, err := svc.CreateRecordGrant(e.Admin(), identity.CreateGrantInput{
		RecordType: "person", RecordID: shared,
		SubjectType: "user", SubjectID: e.Rep1, Access: "write",
	}); err != nil {
		t.Fatalf("sharing the person at write: %v", err)
	}
	myDeal := e.SeedDeal(t, "My deal", pipeline, open, &e.Rep1)
	theirDeal := e.SeedDeal(t, "Their deal", pipeline, open, &e.Rep3)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	// person
	for _, row := range []struct {
		name string
		id   ids.UUID
		want bool
	}{
		{"a person they own", mine, true},
		{"a colleague's person", theirs, false},
		{"a person shared with them at write", shared, true},
	} {
		got, err := e.People.GetPerson(rep, ids.From[ids.PersonKind](row.id), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("reading %s: %v", row.name, err)
		}
		if got.Writable == nil {
			t.Fatalf("%s came back with no writable flag at all — absent means not writable to a "+
				"client, so a row that IS writable would lose its edit affordances", row.name)
		}
		if *got.Writable != row.want {
			t.Errorf("%s reports writable=%v, want %v", row.name, *got.Writable, row.want)
		}
		// The flag is only worth anything if it agrees with the gate. Drive the
		// mutation and compare: a flag that said yes to a write the server
		// refuses is the defect this field was added to prevent.
		_, err = e.People.UpdatePerson(rep, ids.From[ids.PersonKind](row.id),
			people.UpdatePersonInput{Title: &title})
		if admitted := err == nil; admitted != row.want {
			t.Errorf("%s says writable=%v but the update was admitted=%v — the flag and the gate "+
				"disagree, and the flag is what a client draws its buttons from",
				row.name, row.want, admitted)
		}
	}

	// deal
	for _, row := range []struct {
		name string
		id   ids.UUID
		want bool
	}{
		{"a deal they own", myDeal, true},
		{"a colleague's deal", theirDeal, false},
	} {
		got, err := e.Deals.GetDeal(rep, ids.From[ids.DealKind](row.id), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("reading %s: %v", row.name, err)
		}
		if got.Writable == nil {
			t.Fatalf("%s came back with no writable flag at all", row.name)
		}
		if *got.Writable != row.want {
			t.Errorf("%s reports writable=%v, want %v", row.name, *got.Writable, row.want)
		}
		_, err = e.Deals.UpdateDeal(rep, ids.From[ids.DealKind](row.id),
			deals.UpdateDealInput{Name: &title})
		if admitted := err == nil; admitted != row.want {
			t.Errorf("%s says writable=%v but the update was admitted=%v",
				row.name, row.want, admitted)
		}
	}
}

// TestAMutationEchoCarriesWritabilityToo guards the case a single GET does not:
// the row a PATCH or a create hands back is a different code path, and absent
// reads as NOT writable — so a missing stamp takes the edit buttons away from
// the owner in the moment right after they saved.
func TestAMutationEchoCarriesWritabilityToo(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, orgWriterPermsFor(t))
	name := "Echo Test GmbH"

	created, err := e.People.CreateOrganization(rep, people.CreateOrganizationInput{
		DisplayName: name, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the organization: %v", err)
	}
	if created.Writable == nil || !*created.Writable {
		t.Errorf("the CREATE echo reports writable=%v for the row its own caller just made and owns",
			created.Writable)
	}

	changed := "Echo Test GmbH II"
	updated, err := e.People.UpdateOrganization(rep, ids.From[ids.OrganizationKind](ids.UUID(created.Id)),
		people.UpdateOrganizationInput{DisplayName: &changed})
	if err != nil {
		t.Fatalf("updating the organization: %v", err)
	}
	if updated.Writable == nil || !*updated.Writable {
		t.Errorf("the UPDATE echo reports writable=%v for a row the same caller just wrote",
			updated.Writable)
	}
}

// TestAnArchivedRecordIsNotWritableOnTheWire holds the state half. The write
// gate's own probes are LIVE-only, so a row the caller owns but has archived is
// refused by every mutation — and a flag that ignored that would offer an edit
// the server declines.
func TestAnArchivedRecordIsNotWritableOnTheWire(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, orgWriterPermsFor(t))
	created, err := e.People.CreateOrganization(rep, people.CreateOrganizationInput{
		DisplayName: "Archived Co", Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the organization: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(created.Id))
	if _, err := e.People.ArchiveOrganization(rep, orgID, nil); err != nil {
		t.Fatalf("archiving: %v", err)
	}
	got, err := e.People.GetOrganization(rep, orgID, storekit.IncludeArchived)
	if err != nil {
		t.Fatalf("reading the archived organization: %v", err)
	}
	if got.Writable != nil && *got.Writable {
		t.Error("an archived organization reports writable=true, but every mutation takes the LIVE " +
			"probe and refuses it — the flag is promising an edit the server declines")
	}
}

// TestAListPageCarriesWritabilityOnEveryRow guards the half a single read
// cannot: the list is a different code path, and a page that reported every row
// writable would put an edit affordance on every colleague's record at once.
func TestAListPageCarriesWritabilityOnEveryRow(t *testing.T) {
	e := Setup(t)
	mine := e.SeedPerson(t, "Mine on the page", &e.Rep1)
	theirs := e.SeedPerson(t, "Theirs on the page", &e.Rep3)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	page, _, err := e.People.ListPeople(rep, people.ListPeopleInput{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[ids.UUID]bool{mine: true, theirs: false}
	seen := 0
	for _, p := range page {
		expect, named := want[ids.UUID(p.Id)]
		if !named {
			continue
		}
		seen++
		if p.Writable == nil {
			t.Errorf("%s came back from the LIST with no writable flag", p.FullName)
			continue
		}
		if *p.Writable != expect {
			t.Errorf("the list reports %s writable=%v, want %v", p.FullName, *p.Writable, expect)
		}
	}
	if seen != len(want) {
		t.Fatalf("the list returned %d of the %d seeded rows — the fixture is not what this asserts on",
			seen, len(want))
	}
}

// orgWriterPermsFor is AccountRepPerms plus organization.update. No shipped rep
// fixture grants it, and several suites read those fixtures as a rep who cannot
// write a company, so widening one there would make them pass while proving
// nothing.
func orgWriterPermsFor(t *testing.T) principal.Permissions {
	t.Helper()
	perms := AccountRepPerms
	perms.Objects = map[string]principal.ObjectGrant{}
	for object, grant := range AccountRepPerms.Objects {
		perms.Objects[object] = grant
	}
	perms.Objects["organization"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return perms
}
