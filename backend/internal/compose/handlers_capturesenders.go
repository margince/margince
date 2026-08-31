// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Senders page: what this product decided about one seat's correspondents,
// and how they overrule it.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// captureSenderHandlers serves one seat's own view of what was decided about
// their senders.
type captureSenderHandlers struct {
	db    *database.DB
	store *capture.SenderOverrideStore
}

// ListCaptureSenders answers every decision made about the caller's senders.
func (h captureSenderHandlers) ListCaptureSenders(w http.ResponseWriter, r *http.Request) {
	decisions, err := capture.SendersFor(r.Context(), h.db)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.CaptureSenderListResponse{
		Data: make([]crmcontracts.CaptureSenderDecision, 0, len(decisions)),
	}
	for _, d := range decisions {
		out.Data = append(out.Data, toContractSenderDecision(d))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// SetCaptureSenderDecision records the caller's own answer about a sender.
func (h captureSenderHandlers) SetCaptureSenderDecision(w http.ResponseWriter, r *http.Request, address string) {
	var req crmcontracts.SetCaptureSenderDecisionJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	saved, err := h.store.Set(r.Context(), address, string(req.Decision))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractSenderDecision(capture.SenderDecision{
		Address:       saved.Address,
		Decision:      saved.Decision,
		OverruledKind: saved.OverruledKind,
	}))
}

// DeleteCaptureSenderDecision withdraws the caller's answer, handing the sender
// back to the classifier.
func (h captureSenderHandlers) DeleteCaptureSenderDecision(w http.ResponseWriter, r *http.Request, address string) {
	if err := h.store.Remove(r.Context(), address); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toContractSenderDecision renders one sender for the page.
//
// The empty strings become absent fields rather than empty ones: "the
// classifier has not answered yet" and "it answered with the empty kind" are
// different facts, and only one of them is true.
func toContractSenderDecision(d capture.SenderDecision) crmcontracts.CaptureSenderDecision {
	out := crmcontracts.CaptureSenderDecision{
		Address:      d.Address,
		Overruled:    d.Overruled(),
		RecordExists: d.RecordExists,
	}
	if d.Kind != "" {
		kind := d.Kind
		out.Kind = &kind
	}
	if d.Status != "" {
		status := d.Status
		out.Status = &status
	}
	if d.Decision != "" {
		decision := crmcontracts.CaptureSenderDecisionDecision(d.Decision)
		out.Decision = &decision
	}
	if d.OverruledKind != "" {
		overruled := d.OverruledKind
		out.OverruledKind = &overruled
	}
	return out
}
