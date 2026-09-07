// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The refusals a send tool makes before anything is staged.
//
// They share one hazard: a send floored to confirm-first is staged, decided by
// a human, and redeemed — and redemption consumes the one-shot approval before
// the send door validates anything. An argument that could never work would
// burn a decision somebody made, which reads as the system losing it.

import "testing"

// TestAMalformedEvidenceIdIsRefusedBeforeStaging holds the evidence half.
func TestAMalformedEvidenceIdIsRefusedBeforeStaging(t *testing.T) {
	t.Parallel()

	for _, bad := range []SendEvidenceArgs{
		{InvoiceID: "not-a-uuid"},
		{ContractID: "12345"},
		{DealID: "01a05500-0000-7000-8000"},
	} {
		if err := requireParsableEvidence(bad); err == nil {
			t.Errorf("%+v was accepted: it would be refused after approval", bad)
		}
	}

	// Every field empty is the ordinary case — evidence is optional, and a
	// caller that names nothing says nothing.
	if err := requireParsableEvidence(SendEvidenceArgs{}); err != nil {
		t.Errorf("naming no evidence was refused: %v", err)
	}
	// A well-formed id passes, or the refusals above would be
	// indistinguishable from a check that refuses everything.
	if err := requireParsableEvidence(SendEvidenceArgs{
		InvoiceID: "01a05500-0000-7000-8000-000000000001",
	}); err != nil {
		t.Errorf("a well-formed evidence id was refused: %v", err)
	}
}

// TestASendWithNoAddresseeIsRefusedBeforeStaging holds the sibling this was
// modelled on, which had no test of its own.
func TestASendWithNoAddresseeIsRefusedBeforeStaging(t *testing.T) {
	t.Parallel()

	if err := requireAddressee(nil); err == nil {
		t.Error("a send with no addressee was accepted: it reaches nobody")
	}
	if err := requireAddressee([]string{}); err == nil {
		t.Error("a send with an empty addressee list was accepted")
	}
	if err := requireAddressee([]string{"dana@buyer.test"}); err != nil {
		t.Errorf("a send with one addressee was refused: %v", err)
	}
}
