// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

// The verdict pass spells its principal as a literal so the system-principal
// census can read it. The display-name origin test in contacts recognises a
// contact the verdict minted by the same value, so the two must agree.
func TestTheVerdictActorIsTheOneContactsReadsAsCapture(t *testing.T) {
	t.Parallel()
	if verdictActor != contacts.CaptureVerdictActor {
		t.Errorf("verdictActor = %q, contacts.CaptureVerdictActor = %q: a contact the verdict mints would read as named by somebody, and its guessed name would never be repaired",
			verdictActor, contacts.CaptureVerdictActor)
	}
	if counterpartyExecutorActor != contacts.CaptureAcceptActor {
		t.Errorf("counterpartyExecutorActor = %q, contacts.CaptureAcceptActor = %q", counterpartyExecutorActor, contacts.CaptureAcceptActor)
	}
}
