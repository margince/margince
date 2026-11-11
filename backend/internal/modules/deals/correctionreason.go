// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"maps"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func correctionReasonEvidence(evidence map[string]any, kind crmcontracts.ReceiptReasonKind) map[string]any {
	out := maps.Clone(evidence)
	if out == nil {
		out = map[string]any{}
	}
	switch kind {
	case crmcontracts.ReceiptReasonKindSystemRule, crmcontracts.ReceiptReasonKindInference:
		out["reason_kind"] = kind
	default:
		delete(out, "reason_kind")
	}
	return out
}
