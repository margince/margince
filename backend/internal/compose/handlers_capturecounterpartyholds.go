// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The counterparty-hold surface: list whose mail the caller keeps to the people
// on it, place a hold, lift one. Thin transport — the capture store owns the
// gate (a human seat, and the row must be theirs) and the audited write.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type captureCounterpartyHoldHandlers struct {
	store *capture.CounterpartyHoldStore
	// The two seams re-opening a hold's history needs, injected here because a
	// module never imports a sibling: the audience derivation and the clearing
	// of the row-level hold both belong to activities.
	recompute capture.AudienceRecomputer
	clearHold capture.CounterpartyHoldClearer
}

func (h captureCounterpartyHoldHandlers) ListCaptureCounterpartyHolds(w http.ResponseWriter, r *http.Request) {
	holds, err := h.store.List(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.CaptureCounterpartyHoldListResponse{
		Data: make([]crmcontracts.CaptureCounterpartyHold, 0, len(holds)),
	}
	for _, hold := range holds {
		out.Data = append(out.Data, toContractCounterpartyHold(hold))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h captureCounterpartyHoldHandlers) CreateCaptureCounterpartyHold(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.CreateCaptureCounterpartyHoldRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	hold, err := h.store.Add(r.Context(), string(req.Kind), req.Value)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, toContractCounterpartyHold(hold))
}

func (h captureCounterpartyHoldHandlers) ShareCaptureCounterpartyHoldHistory(w http.ResponseWriter, r *http.Request) {
	released, err := h.store.ShareHistory(r.Context(), h.recompute, h.clearHold)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ShareCaptureHoldHistoryResponse{
		Released: released,
	})
}

func (h captureCounterpartyHoldHandlers) DeleteCaptureCounterpartyHold(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.Remove(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toContractCounterpartyHold maps a stored hold onto the wire shape.
func toContractCounterpartyHold(h capture.CounterpartyHold) crmcontracts.CaptureCounterpartyHold {
	id := openapi_types.UUID(h.ID)
	created := h.CreatedAt
	return crmcontracts.CaptureCounterpartyHold{
		Id:        &id,
		Kind:      crmcontracts.CaptureCounterpartyHoldKind(h.Kind),
		Value:     h.Value,
		CreatedAt: &created,
	}
}
