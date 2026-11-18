// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The degraded flag must reach the wire in both states: a client that reads an
// absent field as false would otherwise show "fine" for a draft whose voice
// was lost, which is the exact silence this field exists to end.
func TestWireCarriesTheVoiceDegradedFlag(t *testing.T) {
	t.Parallel()
	draft := Draft{Subject: "s", Body: "b"}

	degraded := Wire(draft, crmcontracts.WrittenByModel, true, "en", "")
	if degraded.VoiceDegraded == nil || !*degraded.VoiceDegraded {
		t.Fatal("a degraded voice load must be stamped on the wire draft")
	}

	clean := Wire(draft, crmcontracts.WrittenByModel, false, "en", "")
	if clean.VoiceDegraded == nil || *clean.VoiceDegraded {
		t.Fatal("a clean load must stamp voice_degraded=false, not omit it")
	}
}

// A draft asked to move an appointment names the requested follow-up, for the
// composer to send with it. Any other instruction names no reason.
func TestWireCarriesTheReasonTheInstructionNames(t *testing.T) {
	t.Parallel()
	draft := Draft{Subject: "s", Body: "b"}
	moved := Wire(draft, crmcontracts.WrittenByModel, false, "de", "Termin verschieben")
	if moved.CommunicationContext == nil || *moved.CommunicationContext != crmcontracts.CommunicationContextRequestedFollowup {
		t.Fatalf("communication_context = %v, want requested_followup", moved.CommunicationContext)
	}
	if other := Wire(draft, crmcontracts.WrittenByModel, false, "en", "send the brochure"); other.CommunicationContext != nil {
		t.Fatalf("an instruction naming no reason gave %v", *other.CommunicationContext)
	}
}
