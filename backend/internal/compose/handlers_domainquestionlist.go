// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func (h domainQuestionHandlers) ListDomainQuestions(w http.ResponseWriter, r *http.Request, params crmcontracts.ListDomainQuestionsParams) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	domain := ""
	if params.Domain != nil {
		domain = *params.Domain
	}
	rows, page, err := h.contacts.DomainQuestionsForOwner(r.Context(), cursor, storekit.ClampLimit(params.Limit), domain)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if domain != "" && len(rows) == 0 {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	data := make([]crmcontracts.DomainQuestion, 0, len(rows))
	for _, q := range rows {
		item := crmcontracts.DomainQuestion{
			Domain: q.Domain, ReasonCode: q.ReasonCode, AskedAt: q.AskedAt, EvidenceAt: q.EvidenceAt, Deferred: q.Deferred,
			DomainContext: attention.DomainQuestionContextWire(q.Context),
		}
		if q.Note != "" {
			item.Note = &q.Note
		}
		if q.Forward != nil {
			item.EffectiveUrl = &q.Forward.EffectiveURL
			item.RedirectChain = &q.Forward.RedirectChain
		}
		data = append(data, item)
	}
	info := crmcontracts.PageInfo{HasMore: page.HasMore}
	if page.NextCursor != "" {
		info.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.DomainQuestionPage{Data: data, Page: info})
}
