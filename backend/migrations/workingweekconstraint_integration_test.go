// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The census in backend/gates/emptyarraycheck_test.go reads the catalog's
// TEXT — it can tell cardinality from array_length, but not whether Postgres
// actually refuses the row the old spelling let through. This is that other
// half: it inserts the one value the constraint exists to refuse.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestAWorkingWeekHasADayInIt proves the behaviour behind the catalog line the
// census checks, in both directions: `{}` is refused, a real week and an
// unset week are not.
func TestAWorkingWeekHasADayInIt(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	_, err := conn.Exec(ctx,
		`INSERT INTO app_user (email, display_name, work_days)
		 VALUES ('empty-week@example.test', 'Empty Week', '{}'::smallint[])`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("inserting work_days = '{}' failed with %v, want a *pgconn.PgError", err)
	}
	if pgErr.Code != "23514" || pgErr.ConstraintName != "app_user_work_days_are_weekdays" {
		t.Fatalf("inserting work_days = '{}' failed with code %q constraint %q, "+
			"want 23514 on app_user_work_days_are_weekdays", pgErr.Code, pgErr.ConstraintName)
	}

	if _, err := conn.Exec(ctx,
		`INSERT INTO app_user (email, display_name, work_days)
		 VALUES ('real-week@example.test', 'Real Week', '{1,2,3,4,5}'::smallint[])`); err != nil {
		t.Errorf("inserting a real week was refused: %v", err)
	}

	if _, err := conn.Exec(ctx,
		`INSERT INTO app_user (email, display_name, work_days)
		 VALUES ('unset-week@example.test', 'Unset Week', NULL)`); err != nil {
		t.Errorf("inserting work_days = NULL was refused, but the constraint is explicitly nullable: %v", err)
	}
}
