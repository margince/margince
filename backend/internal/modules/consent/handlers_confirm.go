// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The no-login confirm-your-details transport. The public middleware has
// already turned an unknown token away and bound the workspace plus a system
// principal; each handler resolves the token again for the person it names —
// the same infra read the preference surface makes — and then drives the store.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// GetConfirmDetails implements (GET /public/confirm/{token}): one contact's own
// view of what is held about them. Resolving stamps the link as opened, which is
// the middle of the ask-to-click chain the proof row later refers to.
func (h Handlers) GetConfirmDetails(w http.ResponseWriter, r *http.Request, token string) {
	ref, err := h.store.ResolveConfirmToken(r.Context(), token)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	card, err := h.store.confirmCardFor(r.Context(), ref.PersonID)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireConfirmCard(card))
}

// SubmitConfirmDetails implements (POST /public/confirm/{token}): the answer
// coming back. The store spends the link and records everything in one
// transaction, so a replayed submit refuses rather than writing twice.
func (h Handlers) SubmitConfirmDetails(w http.ResponseWriter, r *http.Request, token string) {
	var req crmcontracts.SubmitConfirmDetailsJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.SubmitConfirmation(r.Context(), token, submissionFromWire(req)); err != nil {
		writeConsentErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// submissionFromWire reads the request into the store's shape. A repeated field
// keeps the LAST value rather than refusing: the page sends at most one box per
// field, so a duplicate is a client bug, and taking the last is what a form post
// would do anyway.
func submissionFromWire(req crmcontracts.SubmitConfirmDetailsJSONRequestBody) ConfirmSubmission {
	in := ConfirmSubmission{Corrections: map[string]string{}}
	if req.Corrections != nil {
		for _, c := range *req.Corrections {
			in.Corrections[string(c.Field)] = c.Value
		}
	}
	if req.RequestErasure != nil {
		in.RequestErasure = *req.RequestErasure
	}
	if req.MarketingChoice != nil {
		in.MarketingChoice = string(*req.MarketingChoice)
	}
	if req.MarketingWording != nil {
		in.MarketingWording = *req.MarketingWording
	}
	return in
}

// wireConfirmCard renders the card. marketing_state is spelled 'unknown' rather
// than empty, matching the preference surface's own vocabulary: no record and a
// withdrawal are different answers, and the page shows them differently.
func wireConfirmCard(card ConfirmCard) crmcontracts.ConfirmDetails {
	state := card.Marketing
	if state == "" {
		state = "unknown"
	}
	origins := make([]struct {
		Field      string `json:"field"`
		RecordedAt string `json:"recorded_at"`
		Source     string `json:"source"`
	}, 0, len(card.Provenance))
	for _, o := range card.Provenance {
		origins = append(origins, struct {
			Field      string `json:"field"`
			RecordedAt string `json:"recorded_at"`
			Source     string `json:"source"`
		}{Field: o.Field, RecordedAt: o.RecordedAt, Source: o.Source})
	}
	return crmcontracts.ConfirmDetails{
		FullName:       card.FullName,
		Title:          card.Title,
		Company:        card.Company,
		Email:          card.Email,
		Phone:          card.Phone,
		MarketingState: crmcontracts.ConfirmDetailsMarketingState(state),
		Provenance:     origins,
	}
}
