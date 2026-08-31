// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"net/http"

	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The two verbs that let a person state and unstate a fact, beside the correct
// and confirm verbs in handlers_organization.go rather than in it: that file
// is near the length ceiling, and the four together are one story a reader
// follows across two short files more easily than one long one.

// CreateOrganizationFact records a fact a person states about a company.
//
// A crawl reads what a page publishes, and most of what a rep knows was never
// published — so without this the fact list could only ever hold what a machine
// happened to find.
func (h Handlers) CreateOrganizationFact(w http.ResponseWriter, r *http.Request,
	id crmcontracts.Id, _ crmcontracts.CreateOrganizationFactParams,
) {
	var req crmcontracts.CreateOrganizationFactRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.CreateOrganizationFact(r.Context(),
		pathID[ids.OrganizationKind](id),
		FactCreateInput{
			Category: string(req.Category),
			Field:    req.Field,
			Value:    req.Value,
		})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// DeleteOrganizationFact removes a fact this company does not state.
//
// 204 rather than the removed row: the row is gone, and answering with a body
// describing it would invite a reader to believe it is still there.
func (h Handlers) DeleteOrganizationFact(w http.ResponseWriter, r *http.Request,
	id crmcontracts.Id, factKey crmcontracts.FactKey,
	_ crmcontracts.DeleteOrganizationFactParams,
) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteOrganizationFact(r.Context(),
		pathID[ids.OrganizationKind](id), factKey,
		FactWriteInput{IfVersion: ifVersion}); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
