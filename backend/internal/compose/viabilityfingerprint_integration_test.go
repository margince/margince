// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fingerprintOf is the deal's viability fingerprint as the forecast reads it.
func (v *viabilityEnv) fingerprintOf(t *testing.T, deal ids.UUID) string {
	t.Helper()
	return v.WsScalar(t, `SELECT `+viabilityFingerprintSQL("d")+` FROM deal d WHERE d.id = $1`, deal)
}

// Every input the viability assessment reads moves the fingerprint, so a
// receipt is not trusted once the evidence behind it changed. Each case changes
// one input after the fingerprint was taken.
func TestEveryViabilityInputMovesTheFingerprint(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, v *viabilityEnv, mail ids.UUID) ids.UUID
	}{
		{"the sender's contact", func(t *testing.T, v *viabilityEnv, mail ids.UUID) ids.UUID {
			other := v.SeedContact(t, "Somebody else", nil)
			v.WsExec(t, `UPDATE activity_participant SET contact_id = $2 WHERE activity_id = $1 AND role = 'from'`, mail, other)
			return v.supported
		}},
		{"the sender's role", func(t *testing.T, v *viabilityEnv, mail ids.UUID) ids.UUID {
			v.WsExec(t, `UPDATE activity_participant SET role = 'to' WHERE activity_id = $1 AND role = 'from'`, mail)
			return v.supported
		}},
		{"a contact the message is filed under", func(t *testing.T, v *viabilityEnv, mail ids.UUID) ids.UUID {
			v.WsExec(t, `DELETE FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, mail, v.buyer)
			return v.supported
		}},
		{"who owns a senderless message's address", func(t *testing.T, v *viabilityEnv, mail ids.UUID) ids.UUID {
			v.WsExec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
				VALUES ($1, 'buyer@client.example', 'manual', 'human:x')`, v.buyer)
			return v.supported
		}},
		{"a guest's answer", func(t *testing.T, v *viabilityEnv, mail ids.UUID) ids.UUID {
			v.WsExec(t, `UPDATE activity_participant SET response = 'declined' WHERE activity_id = $1`, mail)
			return v.supported
		}},
		{"the next step's state", func(t *testing.T, v *viabilityEnv, _ ids.UUID) ids.UUID {
			v.WsExec(t, `UPDATE activity SET is_done = true, done_at = now() FROM activity_link l
				WHERE l.activity_id = activity.id AND l.deal_id = $1 AND activity.kind = 'task'`, v.supported)
			return v.supported
		}},
		{"an answer to the earlier check", func(t *testing.T, v *viabilityEnv, _ ids.UUID) ids.UUID {
			until := v.now.AddDate(0, 0, 30)
			v.answerEarlierCheck(t, assurance.Resolution{
				Outcome: assurance.OutcomeValueCorrect, Reason: "The buyer is on leave", ExpiresAt: &until,
			})
			return v.silent
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := setupViability(t)
			mail := v.customerMails(t, v.supported, v.now.AddDate(0, 0, -2))
			v.WsExec(t, `UPDATE activity SET counterparty_email = 'buyer@client.example' WHERE id = $1`, mail)
			before := map[ids.UUID]string{v.supported: v.fingerprintOf(t, v.supported), v.silent: v.fingerprintOf(t, v.silent)}

			deal := tc.change(t, v, mail)

			if after := v.fingerprintOf(t, deal); after == before[deal] {
				t.Errorf("changing %s left the fingerprint at %s, so a receipt keeps vouching for evidence that moved",
					tc.name, after)
			}
		})
	}
}
