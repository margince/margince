// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SeedPassport inserts a live passport for Rep1 and returns its id. Rows that
// reference a passport carry a real foreign key. So the database, not the code
// under test, would reject a synthetic id.
func (e *Env) SeedPassport(t *testing.T, owner *pgx.Conn, label string) ids.UUID {
	t.Helper()
	return e.SeedPassportFor(t, owner, label, e.Rep1)
}

// SeedPassportFor is SeedPassport for the user the agent acts for.
func (e *Env) SeedPassportFor(t *testing.T, owner *pgx.Conn, label string, human ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO passport (id, on_behalf_of, granted_by, label, scopes, token_hash, expires_at)
		VALUES ($1, $2, $2, $3, ARRAY['read','write'], $4, now() + interval '1 day')`,
		id, human, label, "hash-"+id.String()); err != nil {
		t.Fatalf("seeding passport %s: %v", label, err)
	}
	return id
}
