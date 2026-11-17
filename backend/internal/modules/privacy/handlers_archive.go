// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListArchivedActivities implements (GET /retention/archive).
func (h Handlers) ListArchivedActivities(w http.ResponseWriter, r *http.Request, params crmcontracts.ListArchivedActivitiesParams) {
	f := ArchiveFilter{Before: params.Before, Cursor: params.Cursor, Limit: params.Limit}
	if params.ContactId != nil {
		contact := ids.UUID(*params.ContactId)
		f.ContactID = &contact
	}
	if params.CompanyId != nil {
		company := ids.UUID(*params.CompanyId)
		f.CompanyID = &company
	}
	page, err := h.eraser.ListArchivedActivities(r.Context(), f)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	data := make([]crmcontracts.ArchivedActivity, 0, len(page.Records))
	for _, record := range page.Records {
		data = append(data, archivedActivityToWire(record))
	}
	resp := struct {
		Data []crmcontracts.ArchivedActivity `json:"data"`
		Page crmcontracts.PageInfo           `json:"page"`
	}{Data: data, Page: crmcontracts.PageInfo{HasMore: page.HasMore}}
	if page.NextCursor != "" {
		resp.Page.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, resp)
}

// archivedActivityToWire states the party lists always, empty meaning the
// record is linked to no one.
func archivedActivityToWire(record ArchivedActivity) crmcontracts.ArchivedActivity {
	out := crmcontracts.ArchivedActivity{
		ActivityId:        openapi_types.UUID(record.ActivityID),
		Kind:              record.Kind,
		Subject:           record.Subject,
		OccurredAt:        record.OccurredAt,
		OccurredPrecision: crmcontracts.OccurredPrecision(record.Precision),
		ArchivedAt:        record.ArchivedAt,
		Contacts:          archivePartiesToWire(record.Contacts),
		Companies:         archivePartiesToWire(record.Companies),
	}
	if record.Kept != "" {
		kept := crmcontracts.ArchiveKeptReason(record.Kept)
		out.Kept = &kept
	}
	return out
}

func archivePartiesToWire(parties []ArchiveParty) []crmcontracts.ArchivedActivityParty {
	out := make([]crmcontracts.ArchivedActivityParty, 0, len(parties))
	for _, party := range parties {
		out = append(out, crmcontracts.ArchivedActivityParty{Id: openapi_types.UUID(party.ID), Name: party.Name})
	}
	return out
}

// DeleteArchivedActivities implements (POST /retention/archive/delete).
func (h Handlers) DeleteArchivedActivities(w http.ResponseWriter, r *http.Request, _ crmcontracts.DeleteArchivedActivitiesParams) {
	var req crmcontracts.ArchiveDeletionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	activityIDs := make([]ids.UUID, 0, len(req.ActivityIds))
	for _, id := range req.ActivityIds {
		activityIDs = append(activityIDs, ids.UUID(id))
	}
	done, err := h.eraser.DeleteArchivedActivities(r.Context(), activityIDs)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.ArchiveDeletionResult{
		Deleted: make([]openapi_types.UUID, 0, len(done.Deleted)),
		Kept:    make([]crmcontracts.ArchiveKeptActivity, 0, len(done.Kept)),
	}
	for _, id := range done.Deleted {
		out.Deleted = append(out.Deleted, openapi_types.UUID(id))
	}
	for _, kept := range done.Kept {
		out.Kept = append(out.Kept, crmcontracts.ArchiveKeptActivity{
			ActivityId: openapi_types.UUID(kept.ActivityID),
			Reason:     crmcontracts.ArchiveKeptReason(kept.Reason),
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
