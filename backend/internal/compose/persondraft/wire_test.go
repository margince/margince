// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package persondraft

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

	degraded := Wire(draft, crmcontracts.Model, true)
	if degraded.VoiceDegraded == nil || !*degraded.VoiceDegraded {
		t.Fatal("a degraded voice load must be stamped on the wire draft")
	}

	clean := Wire(draft, crmcontracts.Model, false)
	if clean.VoiceDegraded == nil || *clean.VoiceDegraded {
		t.Fatal("a clean load must stamp voice_degraded=false, not omit it")
	}
}
