// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The capture exclusion surface: list the rules that bind the caller's
// connections, add one, lift one. Thin transport — the capture store owns the
// gate (admin/ops for a workspace rule, the user themselves for their own) and
// the audited write.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type captureExclusionHandlers struct {
	store  *capture.ExclusionStore
	purger *CapturePurger
}

func (h captureExclusionHandlers) ListCaptureExclusions(w http.ResponseWriter, r *http.Request) {
	rules, err := h.store.List(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.CaptureExclusionListResponse{Data: make([]crmcontracts.CaptureExclusion, 0, len(rules))}
	for _, rule := range rules {
		out.Data = append(out.Data, toContractExclusion(rule))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h captureExclusionHandlers) CreateCaptureExclusion(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.CreateCaptureExclusionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	rule, err := h.store.Add(r.Context(), string(req.Scope), string(req.Kind), req.Value)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, toContractExclusion(rule))
}

func (h captureExclusionHandlers) DeleteCaptureExclusion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.Remove(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toContractExclusion(rule capture.Exclusion) crmcontracts.CaptureExclusion {
	return crmcontracts.CaptureExclusion{
		Id:        openapi_types.UUID(rule.ID),
		Scope:     crmcontracts.CaptureExclusionScope(rule.Scope),
		Kind:      crmcontracts.CaptureExclusionKind(rule.Kind),
		Value:     rule.Value,
		CreatedAt: rule.CreatedAt,
	}
}

// PurgeCaptureExclusion destroys the mail one exclusion rule already let in.
//
// The rule the exclusion states is about the FUTURE; this is the past it cannot
// reach. Irreversible, which is why the preview answers the same question and
// changes nothing: the counts a caller sees before they confirm are the counts
// they get, because both arms run the same selection.
//
// The rule's own scope decides how far it reaches and who may ask — a seat's own
// rule destroys what that seat imported, a workspace rule destroys what the
// workspace captured and takes the admin role. Purge answers both, so this
// handler passes the id through rather than deciding here.
func (h captureExclusionHandlers) PurgeCaptureExclusion(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.PurgeCaptureExclusionParams,
) {
	if h.purger == nil {
		// A role composed without an object store has no purge: destroying the
		// rows and leaving the attachment files would report mail as gone while
		// its files sat in the bucket. Saying so is better than a nil deref.
		httperr.ServiceUnavailable(w, r,
			"this installation stores no objects, so captured mail cannot be destroyed here")
		return
	}
	preview := params.Preview != nil && *params.Preview
	outcome, err := h.purger.Purge(r.Context(), ids.UUID(id), preview)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.CapturePurgeOutcome{
		Destroyed:  outcome.Destroyed,
		Released:   outcome.Released,
		Skipped:    outcome.Skipped,
		Anonymised: outcome.Anonymised,
		Preview:    outcome.Preview,
	})
}
