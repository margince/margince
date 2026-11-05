// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AssessNoticeSource records the signed-in officer's evidence for the source.
func (h Handlers) AssessNoticeSource(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.AssessNoticeSource
	if !httperr.Decode(w, r, &req) {
		return
	}
	if !req.Kind.Valid() {
		writeConsentErr(w, r, &ValidationError{Field: auditKeyKind, Reason: "confirm how the contact was obtained"})
		return
	}
	updated, err := h.store.AssessNoticeSource(r.Context(), ids.UUID(id), sourceAssessmentOf(req, h.store.now().UTC()))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	h.writeNoticeCase(w, r, updated)
}

func sourceAssessmentOf(req crmcontracts.AssessNoticeSource, now time.Time) SourceAssessment {
	in := SourceAssessment{Kind: string(req.Kind), ObtainedAt: req.ObtainedAt, Note: req.Note, Timezone: valueOr(req.Timezone), Now: now, PreviewToken: valueOr(req.PreviewToken), MetInPerson: req.MetInPerson != nil && *req.MetInPerson}
	if req.SelectedEvidence != nil {
		in.SelectedEvidence = &SourceEvidenceReference{Kind: string(req.SelectedEvidence.Kind), ID: ids.UUID(req.SelectedEvidence.Id)}
	}
	return in
}

// PreviewNoticeSource answers the assessment that confirmation rechecks.
func (h Handlers) PreviewNoticeSource(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.AssessNoticeSource
	if !httperr.Decode(w, r, &req) {
		return
	}
	preview, err := h.store.PreviewNoticeSource(r.Context(), ids.UUID(id), sourceAssessmentOf(req, h.store.now().UTC()))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, preview)
}

// GetNoticeSourceEvidence lists accessible records without withheld placeholders.
func (h Handlers) GetNoticeSourceEvidence(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetNoticeSourceEvidenceParams) {
	evidence, err := h.store.GetNoticeSourceEvidence(r.Context(), ids.UUID(id), valueOr(params.Cursor))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, evidence)
}
