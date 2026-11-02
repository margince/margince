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
