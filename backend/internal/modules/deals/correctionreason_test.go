// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"testing"
)

func TestCorrectionReasonKindComesFromTheWriter(t *testing.T) {
	evidence := map[string]any{"reason_kind": "inference", "basis": "The recorded reason"}
	classified := correctionReasonEvidence(evidence, crmcontracts.ReceiptReasonKindSystemRule)
	if classified["reason_kind"] != crmcontracts.ReceiptReasonKindSystemRule || classified["basis"] != evidence["basis"] {
		t.Fatalf("classified reason = %v", classified)
	}
	if evidence["reason_kind"] != "inference" {
		t.Fatal("classification changed its input")
	}
	unknown := correctionReasonEvidence(evidence, "")
	if _, classified := unknown["reason_kind"]; classified {
		t.Fatalf("an unknown writer inherited the supplied kind: %v", unknown)
	}
}
