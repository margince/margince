// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// ChangeRestorer puts one audited change back. It is a port because the write
// is the RECORD's own update path, which lives in six modules a module may not
// reach; compose owns the seam and injects it. Writing the update here instead
// would be a second write engine, and two writers of one invariant is how the
// audited path and the reversal path come to disagree about what a write means.
type ChangeRestorer interface {
	Restore(ctx context.Context, entityType string, id, auditID ids.UUID, ifVersion int64) (RecordHistoryEntry, error)
}

// WithChangeRestorer wires the reversal seam. Without one the route refuses
// rather than half-serving: an install that has not been given the seam cannot
// answer whether a change is reversible, and saying nothing about it is
// honest where a 500 would not be.
func (h Handlers) WithChangeRestorer(restorer ChangeRestorer) Handlers {
	h.restorer = restorer
	return h
}

// RestoreRecordChange implements
// (POST /records/{entity_type}/{id}/history/{audit_id}/restore).
//
// entityType arrives as a bare, generator-unvalidated string, so this handler
// is its enforcement point exactly as GetRecordHistory is.
func (h Handlers) RestoreRecordChange(w http.ResponseWriter, r *http.Request,
	entityType string, id crmcontracts.Id, auditID openapi_types.UUID,
	params crmcontracts.RestoreRecordChangeParams,
) {
	if !fieldHistoryEntityTypes[entityType] {
		httperr.Write(w, r, httperr.Validation("entity_type", "invalid_entity_type",
			"entity_type must be one of "+fieldHistoryEntityTypeList))
		return
	}
	if h.restorer == nil {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	// Human-only, and checked here rather than left to the record's own write
	// gate: an agent holding write authority on the record would otherwise pass
	// every later check and land a `restore` row declaring a person's change
	// undone. Undoing is an act of authority over what somebody else decided,
	// which is why the contract reserves it to a human.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// If-Match is REQUIRED on this route alone. A restore is decided from a
	// history screen the person has been reading, so the record may have moved
	// under them between reading and pressing; last-write-wins is not an
	// acceptable default for a write whose entire premise is a prior state.
	version, err := strconv.ParseInt(params.IfMatch, 10, 64)
	if err != nil {
		httperr.Write(w, r, httperr.Validation("If-Match", "invalid_if_match",
			"If-Match must be the record's last-seen version"))
		return
	}

	entry, err := h.restorer.Restore(r.Context(), entityType, ids.UUID(id), ids.UUID(auditID), version)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, recordHistoryEntryToWire(entry))
}
