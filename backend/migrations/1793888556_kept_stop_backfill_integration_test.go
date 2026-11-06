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

// A stop an older anonymize pinned to a plaintext address gains its hash, the
// one the send check computes (storekit.SuppressionHash). This migration keeps
// the address and the next one clears it. A hard bounce gains no hash. Each stop learns how it was recorded from the
// first row of its carry chain, and a chain whose first row is gone reads
// unknown.
func TestTheKeptStopBackfillHashesTheAddressAndReadsTheChain(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	through := namespaceThroughVersion(t, core, "1793888557")
	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, through, 2); err != nil {
		t.Fatalf("rolling the kept-stop columns back: %v", err)
	}

	contact := scalar(t, conn, `INSERT INTO contact (full_name, source, captured_by)
		VALUES ('Dana', 'manual', 'human:x') RETURNING id`)
	stop := func(contactID *string, address *string, kind, source, by string, from *string) string {
		return scalar(t, conn, `
			INSERT INTO communication_suppression (contact_id, address, kind, source, captured_by, decided_by_level, carried_from)
			VALUES ($1, $2, $3, $4, $5, 'subject', $6) RETURNING id`, contactID, address, kind, source, by, from)
	}
	keptAddress, bounceAddress := " Kept@Old.test", "dead@old.test"
	kept := stop(nil, &keptAddress, "marketing_objection", "recorded by a contact", "human:rep", nil)
	bounce := stop(nil, &bounceAddress, "hard_bounce", "bounce", "system", nil)
	typed := stop(&contact, nil, "marketing_objection", "recorded by a contact: said stop", "human:rep", nil)
	carried := stop(&contact, nil, "marketing_objection", "carried: recorded by a contact: said stop", "human:merger", &typed)
	pressed := stop(&contact, nil, "marketing_objection", "public_link", "system", nil)
	orphan := stop(&contact, nil, "marketing_objection", "carried: recorded by a contact", "human:merger", nil)

	if _, err := dbmigrate.Up(ctx, conn, through); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	var address, hash *string
	if err := conn.QueryRow(ctx, `SELECT address, address_hash FROM communication_suppression WHERE id = $1`,
		kept).Scan(&address, &hash); err != nil {
		t.Fatal(err)
	}
	if address == nil || *address != keptAddress || hash == nil || *hash != storekit.SuppressionHash(keptAddress) {
		t.Errorf("the kept stop holds address %v and hash %v, want the address and the send check's hash", address, hash)
	}
	if err := conn.QueryRow(ctx, `SELECT address, address_hash FROM communication_suppression WHERE id = $1`,
		bounce).Scan(&address, &hash); err != nil {
		t.Fatal(err)
	}
	if address == nil || *address != bounceAddress || hash != nil {
		t.Errorf("the hard bounce holds address %v and hash %v, want its address and no hash", address, hash)
	}
	for id, want := range map[string]string{
		typed: "colleague", carried: "colleague", pressed: "subject", orphan: "unknown", bounce: "machine",
	} {
		if got := scalar(t, conn, `SELECT recorded_through FROM communication_suppression WHERE id = $1`, id); got != want {
			t.Errorf("stop %s reads recorded_through %q, want %q", id, got, want)
		}
	}

	// A rollback leaves the plaintext address the older code matches.
	if _, err := dbmigrate.Down(ctx, conn, through, 2); err != nil {
		t.Fatalf("rolling back: %v", err)
	}
	if got := scalar(t, conn, `SELECT address FROM communication_suppression WHERE id = $1`, kept); got != keptAddress {
		t.Errorf("after a rollback the kept stop holds %q, want %q", got, keptAddress)
	}
}
