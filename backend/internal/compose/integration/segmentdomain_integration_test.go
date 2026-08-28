// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The domain leaf, against Postgres.
//
// Its whole content is SQL a unit test cannot judge: whether the correlated
// subquery binds to the account it is filtering, and whether the archived
// predicate inside the wrapper actually excludes a removed domain. Both are
// invisible to a test that reads the template as a string — a leaf correlating
// to nothing selects every account and reads as a working filter until somebody
// notices their segment holds the whole book.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedDomain gives an account one domain, live or removed. Written through the
// owner connection because the filter's subject is the ROW, and what puts it
// there is not what this test is about.
func seedDomain(t *testing.T, owner *pgx.Conn, org ids.UUID, domain string, removed bool) {
	t.Helper()
	archived := "NULL"
	if removed {
		archived = "now()"
	}
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO organization_domain (organization_id, domain, source, captured_by, archived_at)
		 VALUES ($1, $2, 'manual', 'human:x', `+archived+`)`, org, domain); err != nil {
		t.Fatalf("seeding the %s domain: %v", domain, err)
	}
}

func TestASegmentSelectsAccountsByTheirDomain(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	store := collections.NewStore(e.DB())
	// The segment is over ACCOUNTS, so the caller needs the account read the
	// person-shaped fixture perms do not carry.
	perms := collectionsPerms()
	grants := map[string]principal.ObjectGrant{}
	for object, grant := range perms.Objects {
		grants[object] = grant
	}
	grants["organization"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	perms.Objects = grants
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	match := e.SeedOrg(t, "Acme", &e.Rep1)
	seedDomain(t, owner, match, "acme.test", false)
	// A second domain on the SAME account: the leaf selects an account holding
	// AT LEAST the named one, so a company that also runs a product site is
	// still the account somebody searched for.
	seedDomain(t, owner, match, "acme-labs.test", false)

	other := e.SeedOrg(t, "Globex", &e.Rep1)
	seedDomain(t, owner, other, "globex.test", false)

	// The account that USED to be at this domain. It must not be selected: a
	// removed domain is a fact the account no longer carries.
	former := e.SeedOrg(t, "Former Acme", &e.Rep1)
	seedDomain(t, owner, former, "acme.test", true)

	// And one with no domain at all, which the EXISTS must simply not match.
	bare := e.SeedOrg(t, "No Domain", &e.Rep1)

	created, err := store.CreateList(rep, collections.CreateListInput{
		Name: "At acme.test", EntityType: "organization", ListType: "dynamic",
		Definition: map[string]any{"field": "domain", "op": "eq", "value": "acme.test"},
	})
	if err != nil {
		t.Fatalf("a segment on the domain leaf was refused: %v", err)
	}

	members := map[ids.UUID]bool{}
	rows, _, err := store.ListMembers(rep, created.ID, 50, "")
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	for _, m := range rows {
		members[m.EntityID] = true
	}

	for _, want := range []struct {
		what string
		id   ids.UUID
		in   bool
	}{
		{"the account at that domain", match, true},
		{"an account at another domain", other, false},
		{"an account whose domain was removed", former, false},
		{"an account with no domain", bare, false},
	} {
		if members[want.id] != want.in {
			t.Errorf("%s: in segment = %t, want %t", want.what, members[want.id], want.in)
		}
	}
}
