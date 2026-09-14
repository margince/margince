// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAllowedByOverrideFlipsToAllowAndNamesTheRow(t *testing.T) {
	id := ids.NewV7()
	d := Decision{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}
	got := d.AllowedByOverride(id)
	if got.Verdict != VerdictAllow {
		t.Errorf("verdict = %q, want allow", got.Verdict)
	}
	if got.ReasonCode != ReasonAllowedByOverride {
		t.Errorf("reason = %q, want %q", got.ReasonCode, ReasonAllowedByOverride)
	}
	if got.OverrideID != id {
		t.Error("override id was not recorded on the decision")
	}
}

func TestAnOverrideAllowedDecisionIsNotItselfOverrulable(t *testing.T) {
	d := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowedByOverride}
	if d.CanBeOverruled() {
		t.Error("an allow must never be overrulable")
	}
}
