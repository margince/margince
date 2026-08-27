// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetRecordHistory implements (GET /records/{entity_type}/{id}/history).
// entityType arrives as a bare, generator-unvalidated string (an inline
// path param, not a named enum type) — this handler is the only
// enforcement point for the fieldHistoryEntityTypes
// vocabulary, same as GetFieldHistory's entity_type query param.
func (h Handlers) GetRecordHistory(w http.ResponseWriter, r *http.Request,
	entityType string, id crmcontracts.Id, params crmcontracts.GetRecordHistoryParams,
) {
	if !fieldHistoryEntityTypes[entityType] {
		httperr.Write(w, r, httperr.Validation("entity_type", "invalid_entity_type",
			"entity_type must be one of "+fieldHistoryEntityTypeList))
		return
	}
	// An unknown verb is refused rather than answered with an empty page: a
	// caller who mistyped `promoted` for `promote` would otherwise read "that
	// never happened to this record", which is a confident answer to a question
	// they did not ask. The vocabulary is auth's own grant map, which must
	// already name every verb the tree records.
	if params.Action != nil && !auth.IsAuditAction(*params.Action) {
		httperr.Write(w, r, httperr.Validation("action", "unknown_action",
			"action must be an audit verb this installation records"))
		return
	}
	f := RecordHistoryFilter{
		EntityType: entityType,
		EntityID:   ids.UUID(id),
		Cursor:     params.Cursor,
		Limit:      params.Limit,
		Action:     params.Action,
	}

	page, err := ListRecordHistory(r.Context(), h.db, f)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}

	auditIDs := make([]ids.UUID, 0, len(page.Entries))
	for _, entry := range page.Entries {
		auditIDs = append(auditIDs, entry.ID)
	}
	answers := h.undoabilityFor(r.Context(), entityType, ids.UUID(id), auditIDs)
	data := make([]crmcontracts.AuditHistoryEntry, 0, len(page.Entries))
	for _, entry := range page.Entries {
		wire := recordHistoryEntryToWire(entry)
		answer, judged := answers[entry.ID]
		wire.Undoable = undoabilityToWire(answer, judged)
		data = append(data, wire)
	}
	resp := crmcontracts.AuditHistoryListResponse{
		Data: data,
		Page: crmcontracts.PageInfo{HasMore: page.HasMore},
	}
	if page.NextCursor != "" {
		resp.Page.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, resp)
}

// recordHistoryEntryToWire mirrors fieldHistoryEntryToWire's conversion
// style: uuid ids pass through openapi_types.UUID, a nullable uuid only
// sets the pointer when present, and before/after — already masked by
// omission in recordHistoryEntry — stay absent (a nil pointer, never an
// empty object) when the store recorded no image for that side, so a
// hidden key can never resurface as a phantom entry on the wire.
func recordHistoryEntryToWire(e RecordHistoryEntry) crmcontracts.AuditHistoryEntry {
	out := crmcontracts.AuditHistoryEntry{
		Id:                openapi_types.UUID(e.ID),
		ActorType:         crmcontracts.AuditHistoryEntryActorType(e.ActorType),
		ActorId:           e.ActorID,
		ActorName:         e.ActorName,
		Action:            e.Action,
		OccurredAt:        e.OccurredAt,
		AuthorizationRule: e.AuthorizationRule,
		OnBehalfOfName:    e.OnBehalfOfName,
		AgentClient:       e.AgentClient,
		Summary:           e.Summary,
	}
	if e.OnBehalfOf != nil {
		onBehalfOf := openapi_types.UUID(*e.OnBehalfOf)
		out.OnBehalfOf = &onBehalfOf
	}
	if e.UndidAuditLogID != nil {
		undid := openapi_types.UUID(*e.UndidAuditLogID)
		out.UndidAuditLogId = &undid
	}
	if e.Before != nil {
		before := e.Before
		out.Before = &before
	}
	if e.After != nil {
		after := e.After
		out.After = &after
	}
	if e.Edge != nil {
		out.Edge = &crmcontracts.HistoryEdge{
			Kind:            e.Edge.Kind,
			OtherEntityType: crmcontracts.HistoryEdgeOtherEntityType(e.Edge.OtherEntityType),
			OtherEntityId:   openapi_types.UUID(e.Edge.OtherEntityID),
			OtherLabel:      e.Edge.OtherLabel,
		}
	}
	return out
}
