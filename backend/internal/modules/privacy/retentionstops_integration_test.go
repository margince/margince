// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// What an anonymize keeps of a subject's stops, against real rows. It keeps only
// the hash of each address, and only the addresses a merge bound a stop to.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// keptStopSubject seeds a contact holding two addresses and answers its id.
func keptStopSubject(ctx context.Context, t *testing.T, tx pgx.Tx) ids.ContactID {
	t.Helper()
	user := ids.NewV7()
	contact := ids.New[ids.ContactKind]()
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Ines Subject', 'manual', 'user:'||$2::text)`, contact, user)
	for _, address := range []string{"ines@anon.test", "ines.other@anon.test"} {
		mustExec(ctx, t, tx,
			`INSERT INTO contact_email (contact_id, email, source, captured_by)
			 VALUES ($1, $2, 'manual', 'user:'||$3::text)`, contact, address, user)
	}
	return contact
}

// keptHashes reads the hashes the live stops nobody owns hold. It fails on a
// merge binding that still lists the subject's addresses, and on a kept stop
// that still holds a plaintext address.
func keptHashes(ctx context.Context, t *testing.T, tx pgx.Tx) map[string]int {
	t.Helper()
	var plaintext int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM communication_suppression
		 WHERE EXISTS (SELECT 1 FROM unnest(recorded_for_addresses) a WHERE a ILIKE 'ines%@anon.test')
		    OR address ILIKE 'ines%@anon.test'`).Scan(&plaintext); err != nil {
		t.Fatalf("reading the stop table for the subject's addresses: %v", err)
	}
	if plaintext != 0 {
		t.Errorf("%d stop row(s) still hold the subject's addresses in plaintext", plaintext)
	}
	rows, err := tx.Query(ctx, `
		SELECT address_hash FROM communication_suppression
		 WHERE contact_id IS NULL AND lead_id IS NULL AND address_hash IS NOT NULL AND revoked_at IS NULL`)
	if err != nil {
		t.Fatalf("reading the kept stops: %v", err)
	}
	kept, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the kept stops: %v", err)
	}
	counted := map[string]int{}
	for _, hash := range kept {
		counted[hash]++
	}
	return counted
}

// stopsOnAddress counts the live stops nobody owns that the send check's
// address match (storekit.StopOnAddressesSQL) finds for one address.
func stopsOnAddress(ctx context.Context, t *testing.T, tx pgx.Tx, address string) int {
	t.Helper()
	lowered, hashes := storekit.StopAddressArgs(address)
	var n int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM communication_suppression s
		 WHERE s.contact_id IS NULL AND s.lead_id IS NULL AND s.revoked_at IS NULL
		   AND `+storekit.StopOnAddressesSQL("s", "$1", "$2"), lowered, hashes).Scan(&n); err != nil {
		t.Fatalf("matching the stops on %s: %v", address, err)
	}
	return n
}

func TestAnAnonymizeKeepsAStopAsTheHashOfEachAddress(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	contact := keptStopSubject(ctx, t, tx)
	mustExec(ctx, t, tx, `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'subject_request', 'operator_ui', 'human:x', 'user')`, contact)

	if err := anonymizeContactRecord(ctx, tx, contact.UUID, &Eraser{payloads: noPayloads{t: t}}); err != nil {
		t.Fatalf("anonymizing the contact: %v", err)
	}

	kept := keptHashes(ctx, t, tx)
	for _, address := range []string{"ines@anon.test", "ines.other@anon.test"} {
		if kept[storekit.SuppressionHash(address)] != 1 {
			t.Errorf("the stop is kept %d time(s) on the hash of %s, want once (kept: %v)",
				kept[storekit.SuppressionHash(address)], address, kept)
		}
	}
	if n := stopsOnAddress(ctx, t, tx, " Ines@Anon.test "); n != 1 {
		t.Errorf("the send check finds %d stop(s) for the returning address, want the kept one", n)
	}
}

// After a wrong merge one human's stop and withdrawal must not land on the
// other human's address. The merge bound both to the record they were made on.
func TestAnAnonymizePinsABoundStopAndWithdrawalOnlyToTheirOwnAddresses(t *testing.T) {
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:retention"})
	tx := subjectColumnsTx(ctx, t)
	contact := keptStopSubject(ctx, t, tx)
	purpose := ids.NewV7()
	mustExec(ctx, t, tx, `
		INSERT INTO consent_purpose (id, key, label, class, requires_double_opt_in)
		VALUES ($1, $2, 'Newsletter', 'marketing', false)`, purpose, "kept-stop-newsletter-"+purpose.String())
	mustExec(ctx, t, tx, `
		INSERT INTO communication_suppression
		    (contact_id, kind, source, captured_by, decided_by_level, recorded_for_addresses)
		VALUES ($1, 'subject_request', 'operator_ui', 'human:x', 'user', ARRAY['ines.other@anon.test'])`, contact)
	mustExec(ctx, t, tx, `
		INSERT INTO contact_consent (contact_id, purpose_id, state, captured_at, recorded_for_addresses)
		VALUES ($1, $2, 'withdrawn', now(), ARRAY['ines.other@anon.test'])`, contact, purpose)

	if err := anonymizeContactRecord(ctx, tx, contact.UUID, &Eraser{payloads: noPayloads{t: t}}); err != nil {
		t.Fatalf("anonymizing the contact: %v", err)
	}

	kept := keptHashes(ctx, t, tx)
	if n := kept[storekit.SuppressionHash("ines@anon.test")]; n != 0 {
		t.Errorf("%d stop(s) landed on the address the merge did not bind them to, want 0", n)
	}
	if n := kept[storekit.SuppressionHash("ines.other@anon.test")]; n != 2 {
		t.Errorf("%d stop(s) kept on the bound address, want the stop and the withdrawal", n)
	}
	var stillBound int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM contact_consent
		 WHERE contact_id = $1 AND cardinality(recorded_for_addresses) > 0`, contact).Scan(&stillBound); err != nil {
		t.Fatalf("reading the withdrawal's binding: %v", err)
	}
	if stillBound != 0 {
		t.Errorf("%d withdrawal(s) still list the anonymized addresses", stillBound)
	}
}

// Erasing a contact an anonymize already emptied moves its kept hashes onto the
// erasure list and clears them from the row. The returning address stays
// refused, the stops kept on those hashes are deleted, and a stop recorded
// later on the record lands on no address.
//
// Mutation: drop the erasure_suppression insert from anonymizeSubjectRows and
// the address is accepted again.
func TestErasingAnAnonymizedContactKeepsItsAddressesRefused(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	contact := keptStopSubject(ctx, t, tx)
	mustExec(ctx, t, tx, `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'subject_request', 'operator_ui', 'human:x', 'user')`, contact)
	if err := anonymizeContactRecord(ctx, tx, contact.UUID, &Eraser{payloads: noPayloads{t: t}}); err != nil {
		t.Fatalf("anonymizing the contact: %v", err)
	}
	if _, err := anonymizeSubjectRows(ctx, tx, contact, nil, nil, "test"); err != nil {
		t.Fatalf("erasing the anonymized contact: %v", err)
	}

	for _, address := range []string{"ines@anon.test", "ines.other@anon.test"} {
		refused, err := storekit.EmailSuppressed(ctx, tx, address)
		if err != nil {
			t.Fatalf("asking the erasure list: %v", err)
		}
		if !refused {
			t.Errorf("%s is accepted again after the erasure", address)
		}
		if n := stopsOnAddress(ctx, t, tx, address); n != 0 {
			t.Errorf("%d stop(s) kept on the hash of %s outlive the erasure", n, address)
		}
	}
	var kept []string
	if err := tx.QueryRow(ctx,
		`SELECT anonymized_address_hashes FROM contact WHERE id = $1`, contact).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != nil {
		t.Errorf("the erased row still holds %d kept hash(es)", len(kept))
	}
}
