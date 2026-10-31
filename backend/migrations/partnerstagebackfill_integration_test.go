// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// The stage list replaces the fixed stage vocabulary without moving a partner.
// Each keeps its stage, and its role becomes its first kind. A partner already
// active or dormant carries the basis "activated before the rule existed". It
// has no date, because nobody recorded one. No row's version or updated_at
// moves, because nobody edited it.
func TestThePartnerStageListKeepsEveryPartnerWhereItStood(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1793383071")); err != nil {
		t.Fatalf("up to the fixed stage list: %v", err)
	}
	seed := func(name string, role *string, stage string) string {
		t.Helper()
		var companyID string
		if err := conn.QueryRow(ctx, `INSERT INTO company (display_name, source, captured_by)
			VALUES ($1, 'manual', 'test') RETURNING id`, name).Scan(&companyID); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
		if _, err := conn.Exec(ctx, `INSERT INTO partner (company_id, partner_role, relationship_stage, source, captured_by)
			VALUES ($1, $2, $3, 'manual', 'test')`, companyID, role, stage); err != nil {
			t.Fatalf("seeding the partner of %s: %v", name, err)
		}
		return companyID
	}
	hosting, consulting := "hosting", "consulting"
	active := seed("Active Hosting GmbH", &hosting, "active_referring")
	candidate := seed("Candidate Consulting AG", &consulting, "contacted")
	roleless := seed("Roleless Research KG", nil, "research")
	stamps := map[string][2]any{}
	for _, id := range []string{active, candidate, roleless} {
		var version int64
		var updated time.Time
		if err := conn.QueryRow(ctx, `SELECT version, updated_at FROM partner WHERE company_id = $1`, id).
			Scan(&version, &updated); err != nil {
			t.Fatal(err)
		}
		stamps[id] = [2]any{version, updated}
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating to head: %v", err)
	}

	var stages, activeMeaning int
	if err := conn.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE meaning = 'active')
		FROM partner_stage WHERE system`).Scan(&stages, &activeMeaning); err != nil {
		t.Fatal(err)
	}
	if stages != 13 || activeMeaning != 3 {
		t.Errorf("seeded %d built-in stages, %d of them active, want 13 and 3", stages, activeMeaning)
	}
	read := func(id string) (kinds []string, stage string, basis *string, activatedAt *time.Time, version int64, updated time.Time) {
		t.Helper()
		if err := conn.QueryRow(ctx, `SELECT partner_kinds, relationship_stage, activation_basis, activated_at, version, updated_at
			FROM partner WHERE company_id = $1`, id).Scan(&kinds, &stage, &basis, &activatedAt, &version, &updated); err != nil {
			t.Fatalf("reading %s: %v", id, err)
		}
		return kinds, stage, basis, activatedAt, version, updated
	}
	kinds, stage, basis, activatedAt, version, updated := read(active)
	if !slices.Equal(kinds, []string{"hosting"}) || stage != "active_referring" ||
		basis == nil || *basis != "recorded_before_ladder" || activatedAt != nil {
		t.Errorf("the active partner reads kinds %v, stage %s, basis %v, activated %v; want [hosting], active_referring, "+
			"recorded_before_ladder with no date", kinds, stage, basis, activatedAt)
	}
	if version != stamps[active][0].(int64) || !updated.Equal(stamps[active][1].(time.Time)) {
		t.Errorf("the backfill moved the active partner's version or updated_at: %d %v", version, updated)
	}
	kinds, stage, basis, _, _, _ = read(candidate)
	if !slices.Equal(kinds, []string{"consulting"}) || stage != "contacted" || basis != nil {
		t.Errorf("the candidate reads kinds %v, stage %s, basis %v; want [consulting], contacted, not activated", kinds, stage, basis)
	}
	kinds, _, _, _, _, _ = read(roleless)
	if len(kinds) != 0 {
		t.Errorf("a partner with no role reads kinds %v, want none", kinds)
	}

	// Down past the partner lead files too, which come after the stage list.
	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1793397379"), 4); err != nil {
		t.Fatalf("rolling the stage list back: %v", err)
	}
	var role *string
	if err := conn.QueryRow(ctx, `SELECT partner_role FROM partner WHERE company_id = $1`, active).Scan(&role); err != nil {
		t.Fatalf("reading the rolled-back role: %v", err)
	}
	if role == nil || *role != "hosting" {
		t.Errorf("the rollback gave the active partner role %v, want hosting", role)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
}
