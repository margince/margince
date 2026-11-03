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
	updated, err := h.store.AssessNoticeSource(r.Context(), ids.UUID(id), SourceAssessment{
		Kind: string(req.Kind), ObtainedAt: req.ObtainedAt, Note: req.Note, Timezone: valueOr(req.Timezone), Now: time.Now().UTC(),
	})
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	h.writeNoticeCase(w, r, updated)
}
