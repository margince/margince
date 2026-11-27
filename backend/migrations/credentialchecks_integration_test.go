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

// TestACredentialOrRateRowTheWritersRefuseIsRefusedByTheDatabase writes each
// row a check exists to refuse. Postgres tests a CHECK before the foreign keys,
// so the parent ids can be random: the refusal must name the check.
func TestACredentialOrRateRowTheWritersRefuseIsRefusedByTheDatabase(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	const codeColumns = `INSERT INTO oauth_authorization_code
		(client_id, code_hash, code_challenge, redirect_uri, user_id, expires_at, scopes)
		VALUES ('c', repeat('a', 64), 'x', 'https://client.test/cb', gen_random_uuid(), now() + interval '1 minute', `
	const passportColumns = `INSERT INTO passport (on_behalf_of, granted_by, token_hash, expires_at, scopes)
		VALUES (gen_random_uuid(), gen_random_uuid(), repeat('a', 64), now() + interval '1 day', `
	const invitationColumns = `INSERT INTO deal_room_invitation
		(participant_id, token_hash, expires_at, captured_by, attempt_no)
		VALUES (gen_random_uuid(), decode(repeat('ab', 32), 'hex'), now() + interval '1 day', 'human:test', `

	for _, c := range []struct{ name, sql, constraint string }{
		{"a zero rate", `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
			VALUES ('USD', 'EUR', 0, current_date)`, "fx_rate_rate_positive"},
		{"a negative rate", `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
			VALUES ('USD', 'EUR', -0.9, current_date)`, "fx_rate_rate_positive"},
		{"a currency priced in itself", `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
			VALUES ('EUR', 'EUR', 1, current_date)`, "fx_rate_pair_distinct"},
		{"a passport with no scope", passportColumns + `'{}')`, "passport_scopes_present"},
		{"a passport with an unknown scope", passportColumns + `'{read,admin}')`, "passport_scopes_vocabulary"},
		{"a passport with a null scope", passportColumns + `ARRAY['read', NULL])`, "passport_scopes_vocabulary"},
		{"a passport carrying offline_access", passportColumns + `'{read,offline_access}')`, "passport_scopes_vocabulary"},
		{"a grant with no scope", `INSERT INTO oauth_grant (client_id, user_id, scopes)
			VALUES ('c', gen_random_uuid(), '{}')`, "oauth_grant_scopes_present"},
		{"a grant with an unknown scope", `INSERT INTO oauth_grant (client_id, user_id, scopes)
			VALUES ('c', gen_random_uuid(), '{read,admin}')`, "oauth_grant_scopes_vocabulary"},
		{"a code with no scope", codeColumns + `'{}')`, "oauth_authorization_code_scopes_present"},
		{"a code with an unknown scope", codeColumns + `'{read,admin}')`, "oauth_authorization_code_scopes_vocabulary"},
		{"an invitation numbered zero", invitationColumns + `0)`, "deal_room_invitation_attempt_no_check"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := refusingConstraint(t, conn, c.sql); got != c.constraint {
				t.Errorf("the row was refused by %q, want %q", got, c.constraint)
			}
		})
	}
}

// refusingConstraint runs one insert in a transaction it rolls back, and
// answers the CHECK that refused it, or fails when nothing did.
func refusingConstraint(t *testing.T, conn *pgx.Conn, sql string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback: %v", err)
		}
	}()
	_, err = tx.Exec(ctx, sql)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgCheckViolation {
		t.Fatalf("the insert answered %v, want a check violation", err)
	}
	return pgErr.ConstraintName
}

// pgCheckViolation is the SQLSTATE Postgres answers a refused CHECK with.
const pgCheckViolation = "23514"
