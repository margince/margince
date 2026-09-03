// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Senders page: what this product decided about one seat's correspondents,
// and how they overrule it.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

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
	decisions, err := capture.SendersFor(r.Context(), h.db, capture.DefaultPersonalPurgeWindows())
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

// ListHeldThreads answers what the caller's mailbox is withholding.
//
// It sits beside the senders list rather than in a file of its own: both are
// this seat's view of what the capture posture decided on their behalf, both
// are human-only and owner-scoped, and both are read from the same db.
func (h captureSenderHandlers) ListHeldThreads(w http.ResponseWriter, r *http.Request) {
	threads, err := capture.HeldThreadsFor(r.Context(), h.db)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.HeldThreadListResponse{
		Data: make([]crmcontracts.HeldThread, 0, len(threads)),
	}
	for _, t := range threads {
		out.Data = append(out.Data, toContractHeldThread(t))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// toContractHeldThread maps one held thread onto the wire.
//
// The empty strings become absent fields rather than empty ones: a thread whose
// opening message was erased carries no subject, and "" would read as a message
// sent with a blank subject line.
func toContractHeldThread(t capture.HeldThread) crmcontracts.HeldThread {
	out := crmcontracts.HeldThread{
		ThreadKey:  t.ThreadKey,
		Status:     t.Status,
		Pending:    t.Pending(),
		Attempts:   t.Attempts,
		HasMessage: t.HasActivity,
	}
	if t.Kind != "" {
		out.Kind = &t.Kind
	}
	if t.Subject != "" {
		out.Subject = &t.Subject
	}
	if t.ActivityID != nil {
		id := openapi_types.UUID(*t.ActivityID)
		out.ActivityId = &id
	}
	out.OccurredAt = t.OccurredAt
	return out
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
	if !d.DeletesAt.IsZero() {
		at := d.DeletesAt
		out.DeletesAt = &at
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
