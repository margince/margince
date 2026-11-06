// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// The upgrade as it runs: rows written before stops held a hash, then the
// migration that hashes them, then the one that clears the plaintext. A stop
// on a bare address, kept by an anonymize or pressed through a link, ends with
// its hash and no address. A hard bounce and a stop on a record keep their
// addresses. The rollback has no address to restore and writes none.
func TestAKeptStopLosesItsPlaintextAddress(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1793936298")); err != nil {
		t.Fatalf("up to the release before stops held a hash: %v", err)
	}
	contact := scalar(t, conn, `INSERT INTO contact (full_name, source, captured_by)
		VALUES ('Dana', 'manual', 'human:x') RETURNING id`)
	stop := func(contactID *string, address, kind, source string) string {
		return scalar(t, conn, `
			INSERT INTO communication_suppression (contact_id, address, kind, source, captured_by, decided_by_level)
			VALUES ($1, $2, $3, $4, 'system', 'subject') RETURNING id`, contactID, address, kind, source)
	}
	cleared := map[string]string{
		stop(nil, "kept@old.test", "marketing_objection", "withdrawal kept when the record was anonymized"): "kept@old.test",
		stop(nil, "pressed@old.test", "marketing_objection", "public_link"):                                 "pressed@old.test",
	}
	untouched := map[string]string{
		stop(nil, "dead@old.test", "hard_bounce", "bounce"):                            "dead@old.test",
		stop(&contact, "own@old.test", "marketing_objection", "recorded by a contact"): "own@old.test",
	}

	through := namespaceThroughVersion(t, core, "1793936314")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	assertCleared := func(when string) {
		t.Helper()
		for id, was := range cleared {
			var address *string
			var hash string
			if err := conn.QueryRow(ctx, `SELECT address, address_hash FROM communication_suppression WHERE id = $1`,
				id).Scan(&address, &hash); err != nil {
				t.Fatal(err)
			}
			if address != nil || hash != storekit.SuppressionHash(was) {
				t.Errorf("%s the stop on %s holds address %v and hash %q, want its hash alone", when, was, address, hash)
			}
		}
		for id, want := range untouched {
			if got := scalar(t, conn, `SELECT address FROM communication_suppression WHERE id = $1`, id); got != want {
				t.Errorf("%s stop %s holds address %q, want %q", when, id, got, want)
			}
		}
	}
	assertCleared("after the migration")

	if _, err := dbmigrate.Down(ctx, conn, through, 1); err != nil {
		t.Fatalf("rolling back: %v", err)
	}
	assertCleared("after a rollback")
}
