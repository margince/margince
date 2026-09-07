// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListStageExitCriteria answers what a stage requires before a deal leaves it.
func (h Handlers) ListStageExitCriteria(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListStageExitCriteriaParams) {
	archived := storekit.LiveOnly
	if params.IncludeArchived != nil && *params.IncludeArchived {
		archived = storekit.IncludeArchived
	}
	criteria, err := h.store.ListStageExitCriteria(r.Context(), pathID[ids.StageKind](id), archived)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if criteria == nil {
		criteria = []crmcontracts.StageExitCriterion{}
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"data": criteria, "page": crmcontracts.PageInfo{}})
}

// CreateStageExitCriterion adds a criterion to a live, non-terminal stage.
func (h Handlers) CreateStageExitCriterion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.CreateStageExitCriterionParams) {
	var req crmcontracts.CreateStageExitCriterionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	criterion, err := h.store.CreateStageExitCriterion(r.Context(), CreateCriterionInput{
		StageID:  pathID[ids.StageKind](id),
		Key:      req.Key,
		Label:    req.Label,
		Kind:     string(req.Kind),
		Required: req.Required,
		Hint:     req.Hint,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, criterion)
}

// UpdateStageExitCriterion edits a criterion's label, kind, requiredness,
// hint or position. The key is not editable.
func (h Handlers) UpdateStageExitCriterion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, criterionID openapi_types.UUID, _ crmcontracts.UpdateStageExitCriterionParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateStageExitCriterionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := UpdateCriterionInput{
		Label:     req.Label,
		Required:  req.Required,
		IfVersion: ifVersion,
	}
	if req.Kind != nil {
		kind := string(*req.Kind)
		in.Kind = &kind
	}
	// A hint the caller sent as null CLEARS it; one they omitted leaves it
	// standing. Both arrive here as a nil pointer, so the raw body is what
	// tells them apart.
	if req.Hint != nil {
		in.SetHint, in.Hint = true, req.Hint
	}
	for _, field := range httperr.ClearedFields(r) {
		if field == "hint" {
			in.SetHint, in.Hint = true, nil
		}
	}
	criterion, err := h.store.UpdateStageExitCriterion(r.Context(),
		pathID[ids.StageKind](id), criterionArg(criterionID), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, criterion)
}

// ArchiveStageExitCriterion removes a criterion from its stage, keeping the
// row so evidence recorded against it stays readable.
func (h Handlers) ArchiveStageExitCriterion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, criterionID openapi_types.UUID, _ crmcontracts.ArchiveStageExitCriterionParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	if err := h.store.ArchiveStageExitCriterion(r.Context(),
		pathID[ids.StageKind](id), criterionArg(criterionID), ifVersion); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// criterionArg types the criterion path parameter, which the generator emits
// as a bare UUID rather than the contract's Id wrapper.
func criterionArg(u openapi_types.UUID) ids.ExitCriterionID {
	return ids.ExitCriterionID{UUID: ids.UUID(u)}
}

// ListStageEvidence answers what has been observed about this deal against
// its stage's exit criteria.
func (h Handlers) ListStageEvidence(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	evidence, err := h.store.ListStageEvidence(r.Context(), pathID[ids.DealKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if evidence == nil {
		evidence = []crmcontracts.StageEvidence{}
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"data": evidence, "page": crmcontracts.PageInfo{}})
}
