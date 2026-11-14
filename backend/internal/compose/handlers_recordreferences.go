// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const maxRecordReferences = 100

type recordReferenceHandlers struct {
	contacts *contacts.Store
	deals    *deals.Store
	projects *projects.Store
}

func (h recordReferenceHandlers) ReadRecordReferences(w http.ResponseWriter, r *http.Request, params crmcontracts.ReadRecordReferencesParams) {
	object := string(params.Kind)
	switch object {
	case "contact", "company", "lead", "deal", "project":
	default:
		httperr.Write(w, r, httperr.Validation("kind", "invalid", "choose contact, company, lead, deal or project"))
		return
	}
	if err := auth.Require(r.Context(), object, principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if len(params.Id) < 1 || len(params.Id) > maxRecordReferences {
		httperr.Write(w, r, httperr.Validation("id", "invalid_count", fmt.Sprintf("name between 1 and %d records per request", maxRecordReferences)))
		return
	}
	want := make([]ids.UUID, 0, len(params.Id))
	seen := make(map[ids.UUID]bool, len(params.Id))
	for _, wireID := range params.Id {
		id := ids.UUID(wireID)
		if !seen[id] {
			want = append(want, id)
			seen[id] = true
		}
	}
	references, err := h.read(r.Context(), object, want)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	byID := make(map[ids.UUID]crmcontracts.RecordReference, len(references))
	for _, reference := range references {
		byID[ids.UUID(reference.Id)] = reference
	}
	ordered := make([]crmcontracts.RecordReference, 0, len(references))
	for _, id := range want {
		if reference, ok := byID[id]; ok {
			ordered = append(ordered, reference)
		}
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.RecordReferenceListResponse{Data: ordered})
}

func (h recordReferenceHandlers) read(ctx context.Context, kind string, want []ids.UUID) ([]crmcontracts.RecordReference, error) {
	switch kind {
	case "contact":
		return h.contacts.ContactReferences(ctx, want)
	case "company":
		return h.contacts.CompanyReferences(ctx, want)
	case "lead":
		return h.contacts.LeadReferences(ctx, want)
	case "deal":
		return h.deals.DealReferences(ctx, want)
	default:
		return h.projects.ProjectReferences(ctx, want)
	}
}
