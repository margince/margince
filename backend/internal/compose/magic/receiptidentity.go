// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"encoding/json"
	"slices"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func receiptIdentity(e entry) openapi_types.UUID {
	if e.EntityType != typeDeal || e.Action != "advance_stage" {
		return openapi_types.UUID(e.ID)
	}
	var evidence map[string]json.RawMessage
	if json.Unmarshal(e.Evidence, &evidence) != nil {
		return openapi_types.UUID(e.ID)
	}
	var approval string
	if json.Unmarshal(evidence[deals.StageApprovalEvidenceKey], &approval) != nil {
		return openapi_types.UUID(e.ID)
	}
	id, err := ids.Parse(approval)
	if err != nil || id.IsZero() {
		return openapi_types.UUID(e.ID)
	}
	return openapi_types.UUID(id)
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
