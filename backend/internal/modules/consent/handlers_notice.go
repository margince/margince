// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The HTTP surface for disclosure duties.
//
// The notice-case table shipped with a worklist lane that could show duties and
// a mail path that could discharge one, and nothing in between: no way to list
// them, claim one, or close one the product cannot discharge by sending
// anything. These three routes are that middle.

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListNoticeCases answers the privacy officer's queue of disclosure duties.
func (h Handlers) ListNoticeCases(w http.ResponseWriter, r *http.Request, params crmcontracts.ListNoticeCasesParams) {
	var states []NoticeState
	if params.State != nil {
		for _, st := range *params.State {
			// Refused HERE as well as in the store, so a caller naming a state
			// that does not exist learns which field was wrong rather than
			// reading an empty page and concluding there are no duties.
			if !st.Valid() {
				writeConsentErr(w, r, &ValidationError{Field: fieldState, Reason: "not a notice-case state"})
				return
			}
			states = append(states, NoticeState(st))
		}
	}
	limit := 0
	if params.Limit != nil {
		limit = int(*params.Limit)
	}
	cases, err := h.store.ListNoticeCases(r.Context(), states, limit)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	data := make([]crmcontracts.NoticeCase, 0, len(cases))
	for _, c := range cases {
		data = append(data, wireNoticeCase(c))
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{keyData: data})
}

// AssignNoticeCase records who is working a disclosure duty.
func (h Handlers) AssignNoticeCase(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.AssignNoticeCase
	if !httperr.Decode(w, r, &req) {
		return
	}
	updated, err := h.store.AssignNoticeCase(r.Context(), ids.UUID(id), ids.UUID(req.OwnerUserId))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireNoticeCase(updated))
}

// ExcuseNoticeCase ends a disclosure duty on a stated ground, or records that
// it was met somewhere this installation did not send from.
func (h Handlers) ExcuseNoticeCase(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.ExcuseNoticeCase
	if !httperr.Decode(w, r, &req) {
		return
	}
	if !req.State.Valid() {
		writeConsentErr(w, r, &ValidationError{
			Field:  fieldState,
			Reason: "excusing a duty says it was provided elsewhere or is exempt with a reason",
		})
		return
	}
	// The clock is read HERE, at the edge, rather than inside the store — the
	// same division OpenNoticeCaseTx makes, and what lets an integration test
	// drive a deadline without waiting for one.
	in := ExcuseInput{
		State: NoticeState(req.State),
		Note:  req.ResolutionNote,
		Now:   time.Now().UTC(),
	}
	updated, err := h.store.ExcuseNoticeCase(r.Context(), ids.UUID(id), in)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireNoticeCase(updated))
}

// wireNoticeCase is the one place a case crosses into the contract shape, so a
// column added to the row cannot reach two readers in two different spellings.
func wireNoticeCase(c NoticeCase) crmcontracts.NoticeCase {
	out := crmcontracts.NoticeCase{
		Id:        openapi_types.UUID(c.ID),
		ContactId: openapi_types.UUID(c.ContactID.UUID),
		Rule:      crmcontracts.NoticeCaseRule(c.Rule),
		DueAt:     c.DueAt,
		State:     crmcontracts.NoticeCaseState(c.State),
		Attempts:  c.Attempts,
		CreatedAt: c.CreatedAt,

		AssignedAt:     c.AssignedAt,
		ResolutionNote: c.ResolutionNote,

		BlockedReason: c.BlockedReason,
		CompletedAt:   c.CompletedAt,
	}
	if c.OwnerUserID != nil {
		owner := openapi_types.UUID(*c.OwnerUserID)
		out.OwnerUserId = &owner
	}
	if c.ResolvedBy != nil {
		resolver := openapi_types.UUID(*c.ResolvedBy)
		out.ResolvedBy = &resolver
	}
	if c.AllowedRoutes != nil {
		routes := c.AllowedRoutes
		out.AllowedRoutes = &routes
	}
	return out
}
