// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const checkViolation = "23514"

// refusedByCheck reports whether the statement fails on a CHECK. Any other
// failure, such as a missing foreign key row, means the CHECK let the row by.
func refusedByCheck(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args ...any) bool {
	t.Helper()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}()
	_, err = tx.Exec(ctx, sql, args...)
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == checkViolation
}

func TestAnApprovalRefusesATargetTypeNoWriterStages(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	const insert = `INSERT INTO approval (kind, proposed_by, proposed_change, diff_hash, target_entity_type, co_target_entity_type)
		VALUES ('k', 'agent:x', '{}', 'h', $1, $2)`
	for _, c := range []struct {
		target, coTarget any
		refused          bool
	}{
		{"deal", nil, false},
		{"deal", "stage", false},
		{"ext_visit_log_visit", nil, false},
		{nil, nil, false},
		{"widget", nil, true},
		{"deal", "widget", true},
		{"ext:visit-log:visit", nil, true},
		{"Deal", nil, true},
	} {
		if got := refusedByCheck(ctx, t, conn, insert, c.target, c.coTarget); got != c.refused {
			t.Errorf("target %v, co-target %v: refused by CHECK = %v, want %v", c.target, c.coTarget, got, c.refused)
		}
	}
}

func TestAcquisitionEvidenceCitesAnActivityOrADeal(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	const insert = `INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by, source_entity_type, source_entity_id)
		VALUES (uuidv7(), 'requested_quote_or_meeting', now(), 'human:x', $1, uuidv7())`
	for _, c := range []struct {
		source  string
		refused bool
	}{{"activity", false}, {"deal", false}, {"note", true}, {"Activity", true}} {
		if got := refusedByCheck(ctx, t, conn, insert, c.source); got != c.refused {
			t.Errorf("source %q: refused by CHECK = %v, want %v", c.source, got, c.refused)
		}
	}
}
