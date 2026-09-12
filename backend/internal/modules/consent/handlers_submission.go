// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The HTTP doors onto what subjects proposed.

import (
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListConfirmSubmissions implements (GET /confirm-submissions).
func (h Handlers) ListConfirmSubmissions(
	w http.ResponseWriter, r *http.Request, params crmcontracts.ListConfirmSubmissionsParams,
) {
	in := ListSubmissionsInput{Resolved: params.Resolved}
	if params.ContactId != nil {
		in.ContactID = ids.From[ids.ContactKind](ids.UUID(*params.ContactId))
	}
	if params.Limit != nil {
		in.Limit = *params.Limit
	}
	subs, err := h.store.ListSubmissions(r.Context(), in)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	out := make([]crmcontracts.ConfirmSubmission, 0, len(subs))
	for _, sub := range subs {
		out = append(out, wireSubmission(sub))
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// ResolveConfirmSubmission implements (POST /confirm-submissions/{id}/resolve).
func (h Handlers) ResolveConfirmSubmission(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID,
) {
	var body crmcontracts.ResolveConfirmSubmissionJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	in := ResolveSubmissionInput{Resolution: string(body.Resolution)}
	if body.Note != nil {
		in.Note = *body.Note
	}
	out, err := h.store.ResolveSubmission(r.Context(), ids.UUID(id), in)
	if errors.Is(err, ErrUnsupportedField) {
		// NAMED, not swallowed. The reviewer accepted a correction to a field
		// this product cannot write through the ordinary update path, and the
		// one outcome nobody could act on would be a silent accept that moved
		// nothing.
		httperr.Write(w, r, httperr.Validation("resolution", "field_not_writable",
			"this correction names a field that has to be changed on the contact itself — "+
				"the decision was not recorded"))
		return
	}
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireSubmission(out))
}

// wireSubmission renders one proposal for the API.
func wireSubmission(sub Submission) crmcontracts.ConfirmSubmission {
	out := crmcontracts.ConfirmSubmission{
		Id:            openapi_types.UUID(sub.ID),
		ContactId:     openapi_types.UUID(sub.ContactID.UUID),
		Kind:          sub.Kind,
		Field:         sub.Field,
		ProposedValue: sub.ProposedValue,
		SubmittedAt:   sub.SubmittedAt,
		ResolvedAt:    sub.ResolvedAt,
		ResolvedBy:    sub.ResolvedBy,
		Note:          sub.Note,
	}
	if sub.ContactName != "" {
		name := sub.ContactName
		out.ContactName = &name
	}
	if sub.CurrentValue != "" {
		current := sub.CurrentValue
		out.CurrentValue = &current
	}
	if sub.Resolution != nil {
		resolution := crmcontracts.ConfirmSubmissionResolution(*sub.Resolution)
		out.Resolution = &resolution
	}
	return out
}
