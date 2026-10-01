// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// backend/gates/phonee164_test.go holds the CHECK's pattern equal to
// values.E164Pattern — one rule, not two spellings. This is the other half: that
// Postgres actually refuses the spelling the rule is about.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// phoneRow names a dangling contact on purpose. A CHECK is evaluated as the tuple
// is formed while a foreign key is an AFTER trigger, so an unnormalised number
// fails on the CHECK before the contact is looked for — and the normalised case
// below reads that DIFFERENT failure as its evidence of having got past it.
const phoneRow = `
	INSERT INTO contact_phone (contact_id, phone, source, captured_by)
	VALUES (uuidv7(), $1, 'manual', 'human:test')`

func TestAStoredPhoneIsAnE164Number(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	// Every one of these is a spelling of a real number that the dedupe lane
	// cannot match, which is the whole cost: the same contact is created twice.
	for _, unnormalised := range []string{
		"+49 30 1234567", // separators the Go seam strips
		"0049301234567",  // the international-dialling spelling of +
		"030 1234567",    // no country prefix at all
		"+0301234567",    // a zero country code
		"+4930",          // too short to be a number
	} {
		_, err := conn.Exec(ctx, phoneRow, unnormalised)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "contact_phone_e164" {
			t.Errorf("storing %q was not refused by contact_phone_e164 (%v) — it would sit in the table "+
				"invisible to dedupe", unnormalised, err)
		}
	}

	// The normalised form gets past the CHECK and dies on the dangling contact.
	// Naming that SECOND failure is what makes this a measurement: "no shape error"
	// would also be true of a row that was stored, or of no check at all.
	_, err := conn.Exec(ctx, phoneRow, "+49301234567")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("a phone naming no contact was not refused at all (%v) — this case only "+
			"says something about the shape check while the row still fails on its contact", err)
	}
	if pgErr.Code != "23503" || pgErr.ConstraintName != "contact_phone_contact_id_fkey" {
		t.Fatalf("a normalised E.164 number failed with code %q constraint %q, want 23503 on "+
			"contact_phone_contact_id_fkey — the shape check refused it or something else did",
			pgErr.Code, pgErr.ConstraintName)
	}
}
