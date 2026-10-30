// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

const companyLifecycleCheck = "company_lifecycle_check"

func lifecycleCheck(t *testing.T, conn *pgx.Conn) (validated bool, definition string) {
	t.Helper()
	if err := conn.QueryRow(context.Background(), `
		SELECT convalidated, pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conname = $1`, companyLifecycleCheck).Scan(&validated, &definition); err != nil {
		t.Fatalf("reading %s: %v", companyLifecycleCheck, err)
	}
	return validated, definition
}

func TestAFreshDatabaseAcceptsOnboardingAndLive(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	for _, stage := range []string{"onboarding", "live"} {
		if _, err := conn.Exec(context.Background(), `
			INSERT INTO company (display_name, lifecycle, source, captured_by)
			VALUES ($1, $2, 'manual', 'test')`, "Fresh "+stage, stage); err != nil {
			t.Errorf("a fresh database refused a %s company: %v", stage, err)
		}
	}
	if validated, _ := lifecycleCheck(t, conn); !validated {
		t.Error("the lifecycle check on a fresh database is NOT VALID")
	}
}

func TestTheLifecycleDownMovesOnboardingAndLiveToCustomer(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1793390426")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	var onboarding, live string
	if err := conn.QueryRow(ctx, `
		WITH a AS (
		  INSERT INTO company (display_name, lifecycle, source, captured_by)
		  VALUES ('Setting Up Co', 'onboarding', 'manual', 'test') RETURNING id
		), b AS (
		  INSERT INTO company (display_name, lifecycle, source, captured_by)
		  VALUES ('In Use Co', 'live', 'manual', 'test') RETURNING id
		)
		SELECT a.id::text, b.id::text FROM a, b`).Scan(&onboarding, &live); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 2); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, id := range []string{onboarding, live} {
		var stage string
		if err := conn.QueryRow(ctx, `SELECT lifecycle FROM company WHERE id = $1`, id).Scan(&stage); err != nil {
			t.Fatal(err)
		}
		if stage != "customer" {
			t.Errorf("the rollback left a company at %s, want customer", stage)
		}
	}
	// The rollback re-adds the old check NOT VALID rather than scan the table
	// under its lock; every row already satisfies it, so a VALIDATE succeeds.
	_, definition := lifecycleCheck(t, conn)
	if _, err := conn.Exec(ctx, `ALTER TABLE company VALIDATE CONSTRAINT company_lifecycle_check`); err != nil {
		t.Errorf("a row the rollback left breaks the old check: %v", err)
	}
	if strings.Contains(definition, "onboarding") || strings.Contains(definition, "'live'") {
		t.Errorf("the rolled-back check still admits the new stages: %s", definition)
	}

	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up again after the rollback: %v", err)
	}
	if validated, definition := lifecycleCheck(t, conn); !validated || !strings.Contains(definition, "onboarding") {
		t.Errorf("after the round trip the check reads validated=%v %s", validated, definition)
	}
}
