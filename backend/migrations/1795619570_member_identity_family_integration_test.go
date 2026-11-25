// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// A member's identity family is the member and every retired account merged
// into it, through any chain of merges. The account a member was merged into
// is not in it. The family reads the same before the function keeps its plan, after it, and
// after a rollback.
func TestAMemberIdentityFamilyReadsTheSameAcrossItsPlanChange(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1795619570")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling back to the SQL function: %v", err)
	}

	user := func(email string) string { return seedAppUserForConnection(t, conn, email) }
	survivor, retired, retiredTwice, loner := user("survivor@family.example"), user("retired@family.example"),
		user("retired-twice@family.example"), user("loner@family.example")
	merge := func(from, into string) {
		if _, err := conn.Exec(ctx, `UPDATE app_user SET merged_into_user_id = $2, status = 'deactivated', removed_at = now()
			WHERE id = $1`, from, into); err != nil {
			t.Fatalf("merging %s into %s: %v", from, into, err)
		}
	}
	merge(retired, survivor)
	merge(retiredTwice, retired)

	want := map[string][]string{
		survivor:     {survivor, retired, retiredTwice},
		retired:      {retired, retiredTwice},
		retiredTwice: {retiredTwice},
		loner:        {loner},
	}
	families := func(stage string) {
		t.Helper()
		for member, family := range want {
			var got []string
			if err := conn.QueryRow(ctx, `SELECT array_agg(id::text ORDER BY id) FROM unnest(member_identity_family($1)) id`, member).Scan(&got); err != nil {
				t.Fatalf("%s: reading the family of %s: %v", stage, member, err)
			}
			if !slices.Equal(got, slices.Sorted(slices.Values(family))) {
				t.Errorf("%s: family of %s = %v, want %v", stage, member, got, family)
			}
		}
	}
	language := func() string {
		var lang string
		if err := conn.QueryRow(ctx, `SELECT l.lanname FROM pg_proc p JOIN pg_language l ON l.oid = p.prolang WHERE p.proname = 'member_identity_family'`).Scan(&lang); err != nil {
			t.Fatalf("reading the function's language: %v", err)
		}
		return lang
	}

	families("before")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	if got := language(); got != "plpgsql" {
		t.Fatalf("after up the family is %s, want plpgsql", got)
	}
	families("after")
	assertOneCallForEveryMember(ctx, t, conn, slices.Collect(maps.Keys(want)))
	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling back: %v", err)
	}
	if got := language(); got != "sql" {
		t.Fatalf("after down the family is %s, want sql", got)
	}
	families("rolled back")
}

// assertOneCallForEveryMember reads every family in one statement: a session
// that reuses the body's plan must still bind each call's own member.
func assertOneCallForEveryMember(ctx context.Context, t *testing.T, conn *pgx.Conn, members []string) {
	t.Helper()
	var distinct int
	if err := conn.QueryRow(ctx, `SELECT count(DISTINCT member_identity_family(m::uuid)) FROM unnest($1::text[]) m`, members).Scan(&distinct); err != nil {
		t.Fatalf("reading every family at once: %v", err)
	}
	if distinct != len(members) {
		t.Errorf("one statement read %d distinct families for %d members", distinct, len(members))
	}
}
