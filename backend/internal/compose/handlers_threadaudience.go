// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The owner's own say over a thread they imported.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// SetThreadAudience records what this owner concluded about a thread and
// re-derives every message of it they imported.
//
// It lives on the Server rather than on the activities handler set because the
// decision spans two modules: capture owns the ledger the owner's answer is
// written to, activities owns the derivation that answer feeds. Neither may
// import the other, so the assembly is compose's.
func (s Server) SetThreadAudience(w http.ResponseWriter, r *http.Request, threadKey string) {
	if s.threadAudience == nil {
		httperr.ServiceUnavailable(w, r, "this installation captures no mail, so there are no threads to share")
		return
	}
	var req crmcontracts.SetThreadAudienceJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	outcome, err := s.threadAudience.Decide(r.Context(), threadKey, req.Share)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// Every field, spelled once. A field-by-field copy is how activity_ids
	// reached the wire as null on its first day: the setter filled it, this
	// literal did not name it, and the client invalidated nothing while every
	// test that stubbed the response passed.
	activityIDs := make([]openapi_types.UUID, 0, len(outcome.ActivityIDs))
	for _, id := range outcome.ActivityIDs {
		activityIDs = append(activityIDs, openapi_types.UUID(id))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ThreadAudienceOutcome{
		Messages:     outcome.Messages,
		Shared:       outcome.Shared,
		HeldByOthers: outcome.HeldByOthers,
		ActivityIds:  activityIDs,
	})
}
