// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fingerprintOf is what a receipt records as the inputs it judged.
func (v *viabilityEnv) fingerprintOf(t *testing.T, deal ids.UUID) string {
	t.Helper()
	return v.WsScalar(t,
		`SELECT `+viabilityFingerprintSQL("d")+` FROM deal d WHERE d.id = $1`, deal)
}

// contactOnTheDeal is a contact the deal's own activities name, which is what
// the assessment reads to decide whether the customer wrote.
func (v *viabilityEnv) contactOnTheDeal(t *testing.T, deal ids.UUID) string {
	t.Helper()
	return v.WsScalar(t, `
		SELECT fp.contact_id::text FROM activity_link fl
		  JOIN activity fa ON fa.id = fl.activity_id
		  JOIN activity_participant fp ON fp.activity_id = fa.id
		 WHERE fl.deal_id = $1 AND fa.archived_at IS NULL AND fp.contact_id IS NOT NULL
		 LIMIT 1`, deal)
}

// An input the assessment reads moves the fingerprint, so a receipt judged
// before the change stops being current.
//
// A receipt that stays current over changed evidence is the worse failure.
// The deal counts as judged and fine until the next nightly pass.
func TestEveryInputTheAssessmentReadsMovesTheFingerprint(t *testing.T) {
	t.Run("a participant's role", func(t *testing.T) {
		v := setupViability(t)
		contact := v.contactOnTheDeal(t, v.supported)
		before := v.fingerprintOf(t, v.supported)
		v.WsExec(t, `UPDATE activity_participant SET role = 'cc' WHERE contact_id = $1`, contact)
		if after := v.fingerprintOf(t, v.supported); after == before {
			t.Error("a changed role left the fingerprint where it was")
		}
	})

	t.Run("which contact owns an address", func(t *testing.T) {
		v := setupViability(t)
		contact := v.contactOnTheDeal(t, v.supported)
		v.WsExec(t, `INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
			 VALUES ($1, $2, 'buyer@customer.example', 'work', true, 'manual', 'human:x')`,
			ids.NewV7(), contact)
		// Seeded, or the edit below would change nothing and the fingerprint
		// would be right to stay where it is.
		if n := v.WsScalar(t, `SELECT count(*)::text FROM contact_email WHERE contact_id = $1`, contact); n != "1" {
			t.Fatalf("the contact holds %s addresses, want 1", n)
		}
		before := v.fingerprintOf(t, v.supported)
		v.WsExec(t, `UPDATE contact_email SET email = 'moved@customer.example' WHERE contact_id = $1`, contact)
		if after := v.fingerprintOf(t, v.supported); after == before {
			t.Error("an edited address left the fingerprint where it was")
		}
	})

	t.Run("an edit to an address that is not the newest", func(t *testing.T) {
		v := setupViability(t)
		contact := v.contactOnTheDeal(t, v.supported)
		older, newer := ids.NewV7(), ids.NewV7()
		for _, row := range []struct {
			id      ids.UUID
			address string
		}{{older, "first@customer.example"}, {newer, "second@customer.example"}} {
			v.WsExec(t, `INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
				 VALUES ($1, $2, $3, 'work', false, 'manual', 'human:x')`, row.id, contact, row.address)
		}
		// The newer row is edited past the older one, so the edit below changes
		// neither the count of rows nor the highest version.
		for range 3 {
			v.WsExec(t, `UPDATE contact_email SET email_type = email_type WHERE id = $1`, newer)
		}
		before := v.fingerprintOf(t, v.supported)
		v.WsExec(t, `UPDATE contact_email SET email = 'first.moved@customer.example' WHERE id = $1`, older)
		if after := v.fingerprintOf(t, v.supported); after == before {
			t.Error("an edit to an address below the highest version left the fingerprint where it was")
		}
	})

	t.Run("a party known by its address alone", func(t *testing.T) {
		v := setupViability(t)
		activity := v.WsScalar(t, `SELECT fl.activity_id::text FROM activity_link fl
			 JOIN activity fa ON fa.id = fl.activity_id
			WHERE fl.deal_id = $1 AND fa.archived_at IS NULL LIMIT 1`, v.supported)
		party := ids.NewV7()
		v.WsExec(t, `INSERT INTO activity_participant (id, activity_id, address, role)
			 VALUES ($1, $2, 'one@outside.example', 'cc')`, party, activity)
		before := v.fingerprintOf(t, v.supported)
		v.WsExec(t, `UPDATE activity_participant SET address = 'another@outside.example' WHERE id = $1`, party)
		if after := v.fingerprintOf(t, v.supported); after == before {
			t.Error("swapping one address-only party for another left the fingerprint where it was")
		}
	})

	t.Run("a guest's response", func(t *testing.T) {
		v := setupViability(t)
		contact := v.contactOnTheDeal(t, v.supported)
		before := v.fingerprintOf(t, v.supported)
		v.WsExec(t, `UPDATE activity_participant SET response = 'declined' WHERE contact_id = $1`, contact)
		if after := v.fingerprintOf(t, v.supported); after == before {
			t.Error("a changed response left the fingerprint where it was")
		}
	})
}
