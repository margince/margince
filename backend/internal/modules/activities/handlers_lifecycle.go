// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func (h Handlers) UpdateActivity(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateActivityParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateActivityRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	activity, err := h.store.UpdateActivity(r.Context(), pathID[ids.ActivityKind](id),
		activityUpdateInput(req, ifVersion))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, activity)
}

// ArchiveActivity retires one activity, honouring If-Match where the caller
// named a version — including the one an approval's release forwards.
func (h Handlers) ArchiveActivity(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ArchiveActivityParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	activity, err := h.store.ArchiveActivity(r.Context(), pathID[ids.ActivityKind](id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, activity)
}

func (h Handlers) RelinkActivity(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.RelinkActivityParams) {
	var req struct {
		EntityType            string   `json:"entity_type"`
		EntityID              ids.UUID `json:"entity_id"`
		ReplaceExistingOfType bool     `json:"replace_existing_of_type"`
	}
	if !httperr.Decode(w, r, &req) {
		return
	}
	// If-Match is how the REST gate forwards the version an auto-executing
	// agent call was admitted on (compose/agentgate.go), and how a human's
	// client states the version it read. Both mean the same thing to the write.
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	activity, err := h.store.RelinkActivity(r.Context(), pathID[ids.ActivityKind](id), RelinkActivityInput{
		EntityType:            req.EntityType,
		EntityID:              req.EntityID,
		ReplaceExistingOfType: req.ReplaceExistingOfType,
		IfVersion:             ifVersion,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, activity)
}

// RelinkThread moves every writable activity of one conversation in one
// transaction; the count and the ids are the answer.
func (h Handlers) RelinkThread(w http.ResponseWriter, r *http.Request, _ crmcontracts.RelinkThreadParams) {
	var req crmcontracts.RelinkThreadRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	// Forwarded rather than dropped, so the store can say a version cannot
	// condition a batch. A header silently ignored is the same failure as a pin
	// silently ignored, which is what #2614 was about.
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	out, err := h.store.RelinkThread(r.Context(), req.ThreadKey, RelinkActivityInput{
		EntityType:            string(req.EntityType),
		EntityID:              ids.UUID(req.EntityId),
		ReplaceExistingOfType: req.ReplaceExistingOfType != nil && *req.ReplaceExistingOfType,
		IfVersion:             ifVersion,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, relinkBatchWire(out))
}

// RelinkActivities moves a named set of activities in one transaction, or
// none of them.
func (h Handlers) RelinkActivities(w http.ResponseWriter, r *http.Request, _ crmcontracts.RelinkActivitiesParams) {
	var req crmcontracts.RelinkActivitiesRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	// Forwarded for the reason RelinkThread's is: the refusal belongs to the
	// store, and a header this door quietly dropped would be a pin nobody
	// applied and nobody was told about.
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	activityIDs := make([]ids.UUID, 0, len(req.ActivityIds))
	for _, id := range req.ActivityIds {
		activityIDs = append(activityIDs, ids.UUID(id))
	}
	out, err := h.store.RelinkActivities(r.Context(), activityIDs, RelinkActivityInput{
		EntityType:            string(req.EntityType),
		EntityID:              ids.UUID(req.EntityId),
		ReplaceExistingOfType: req.ReplaceExistingOfType != nil && *req.ReplaceExistingOfType,
		IfVersion:             ifVersion,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, relinkBatchWire(out))
}

// relinkBatchWire projects the store's answer onto the contract shape.
func relinkBatchWire(out RelinkBatchResult) crmcontracts.RelinkBatchResult {
	return crmcontracts.RelinkBatchResult{Relinked: out.Relinked}
}

func (h Handlers) SetActivityAudience(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.SetActivityAudienceParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.SetActivityAudienceRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := SetAudienceInput{Audience: string(req.Audience), IfVersion: ifVersion}
	if req.Members != nil {
		for _, m := range *req.Members {
			in.Members = append(in.Members, AudienceMember{SubjectType: string(m.SubjectType), SubjectID: ids.UUID(m.SubjectId)})
		}
	}
	activity, err := h.store.SetAudience(r.Context(), pathID[ids.ActivityKind](id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, activity)
}

// SetActivityDisposition puts a waiting message down.
//
// The three judgements route to two stores because they bind differently:
// `not_sales` settles the thread for everybody, while `snooze` and `not_mine`
// are the caller's own. The transport keeps that split rather than flattening
// it into one write with a scope flag, so a reader of this function can see
// which decisions reach past the person making them.
func (h Handlers) SetActivityDisposition(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.SetActivityDispositionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	// A moment and a reopen condition belong to a snooze and to nothing else.
	// Accepting either on a judgement that does not expire would leave the rep
	// believing their not-mine lifts on Thursday, and nothing would ever tell
	// them otherwise. WHICH shape a snooze itself must take is the store's to
	// judge, because the two tables that hold set-asides answer it identically
	// and a copy here would be the half that drifts.
	isSnooze := req.Disposition == crmcontracts.SetActivityDispositionRequestDispositionSnooze
	if !isSnooze && (req.SnoozedUntil != nil || req.ReopenOn != nil || req.ReopenRef != nil) {
		httperr.Write(w, r, momentMismatch(false))
		return
	}
	activityID := pathID[ids.ActivityKind](id)
	var err error
	switch req.Disposition {
	case crmcontracts.SetActivityDispositionRequestDispositionNotSales:
		err = h.store.SetThreadNotSales(r.Context(), activityID)
	case crmcontracts.SetActivityDispositionRequestDispositionNotMine:
		err = h.store.SetMessageNotMine(r.Context(), activityID)
	case crmcontracts.SetActivityDispositionRequestDispositionSnooze:
		err = h.snoozeFromRequest(r, activityID, req)
	default:
		// An unknown or absent disposition. Without this the switch matches
		// nothing, err stays nil, and the caller is told 204 — that their
		// judgement was recorded — for a request no store ever saw.
		err = &values.ParseError{
			Field: "disposition", Code: "unknown_disposition",
			Message: "a disposition is one of the values the contract lists",
		}
	}
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fieldSnoozedUntil is the contract's spelling of when a snooze lifts. Both
// refusals below point at it, and a caller matching on the field name cannot
// tell a typo from a different field.
const fieldSnoozedUntil = "snoozed_until"

// momentMismatch names which way the moment was wrong, because "invalid" would
// leave a client guessing whether to add one or drop one.
func momentMismatch(wanted bool) error {
	if wanted {
		return &values.ParseError{
			Field: fieldSnoozedUntil, Code: "snooze_needs_a_moment",
			Message: "a snooze names when it lifts",
		}
	}
	return &values.ParseError{
		Field: fieldSnoozedUntil, Code: "moment_not_applicable",
		Message: "only a snooze names a moment; this judgement does not expire",
	}
}

// ClearActivityDisposition picks a message back up.
func (h Handlers) ClearActivityDisposition(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ClearActivityDispositionParams,
) {
	activityID := pathID[ids.ActivityKind](id)
	// The default is the caller's own set-aside. Withdrawing the thread's
	// not-sales judgement is a different reach and has to be asked for by name,
	// so an undo button cannot re-admit a thread a colleague ruled out.
	scope := crmcontracts.ClearActivityDispositionParamsScopeMine
	if params.Scope != nil {
		scope = *params.Scope
	}
	var err error
	switch scope {
	case crmcontracts.ClearActivityDispositionParamsScopeMine:
		err = h.store.ClearMessageDisposition(r.Context(), activityID)
	case crmcontracts.ClearActivityDispositionParamsScopeThread:
		err = h.store.ClearThreadNotSales(r.Context(), activityID)
	default:
		// A scope the contract does not list. Treating it as `mine` would
		// answer 204 to somebody who asked to withdraw the THREAD's judgement
		// and quietly do something narrower.
		err = &values.ParseError{
			Field: "scope", Code: "unknown_scope",
			Message: "a scope is mine or thread",
		}
	}
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PinWorklistRow puts a row at the top of the calling reader's own day.
func (h Handlers) PinWorklistRow(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.WorklistPinRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.PinWorklistRow(r.Context(),
		WorklistRowRef{Source: req.Source, RowID: req.RowId}); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnpinWorklistRow gives the row back to the ranking.
//
// The row is named in the QUERY rather than in a body, because a DELETE with a
// body is a shape half the tooling between here and the browser will drop.
func (h Handlers) UnpinWorklistRow(
	w http.ResponseWriter, r *http.Request, params crmcontracts.UnpinWorklistRowParams,
) {
	if err := h.store.UnpinWorklistRow(r.Context(),
		WorklistRowRef{Source: params.Source, RowID: params.RowId}); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// snoozeFromRequest turns the wire's three optional snooze fields into the
// store's call, refusing a condition outside the set before any of it is
// written.
func (h Handlers) snoozeFromRequest(
	r *http.Request, id ids.ActivityID, req crmcontracts.SetActivityDispositionRequest,
) error {
	var raw *string
	if req.ReopenOn != nil {
		on := string(*req.ReopenOn)
		raw = &on
	}
	on, err := values.ParseReopenCondition(raw, "reopen_on")
	if err != nil {
		return err
	}
	var ref *ids.UUID
	if req.ReopenRef != nil {
		named := ids.UUID(*req.ReopenRef)
		ref = &named
	}
	return h.store.SnoozeMessage(r.Context(), id, on, req.SnoozedUntil, ref)
}
