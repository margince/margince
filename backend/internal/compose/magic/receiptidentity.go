// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"encoding/json"
	"slices"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// receiptIdentity is the Receipt.id an audit row stands for: the approval whose
// effect wrote it, where the row names one, and otherwise the row itself.
func receiptIdentity(e entry) openapi_types.UUID {
	var evidence map[string]json.RawMessage
	if json.Unmarshal(e.Evidence, &evidence) != nil {
		return openapi_types.UUID(e.ID)
	}
	if id, ok := linkedApproval(evidence[storekit.EvidenceKeyReleasedBy]); ok {
		return openapi_types.UUID(id)
	}
	if e.EntityType != typeDeal || e.Action != "advance_stage" {
		return openapi_types.UUID(e.ID)
	}
	if id, ok := linkedApproval(evidence[deals.StageApprovalEvidenceKey]); ok {
		return openapi_types.UUID(id)
	}
	return openapi_types.UUID(e.ID)
}

func linkedApproval(raw json.RawMessage) (ids.UUID, bool) {
	var approval string
	if json.Unmarshal(raw, &approval) != nil {
		return ids.Nil, false
	}
	id, err := ids.Parse(approval)
	if err != nil || id.IsZero() {
		return ids.Nil, false
	}
	return id, true
}

func appendReceiptIdentities(existing, incoming *[]openapi_types.UUID) *[]openapi_types.UUID {
	if incoming == nil {
		return existing
	}
	if existing == nil {
		cloned := slices.Clone(*incoming)
		return &cloned
	}
	for _, id := range *incoming {
		if !slices.Contains(*existing, id) {
			*existing = append(*existing, id)
		}
	}
	return existing
}
