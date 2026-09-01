// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// listCap bounds the history one contact's page returns. An ask is a rare
// event against one contact, so a page that needs more than this is a defect
// rather than a busy account.
const listCap = 50

// askWindow is how long a colleague has before an unanswered ask goes stale.
// A week is a working week: long enough that a holiday does not kill a route,
// short enough that a requester is not left guessing.
const askWindow = 7 * 24 * time.Hour

// Handlers is the introductions transport.
//
// Five verbs, all of them thin: every rule about who may do what lives in the
// store and the lifecycle, so nothing here decides anything. What this layer
// owns is the shape on the wire and the one refusal a body can earn before it
// reaches a store — a route that names an intermediary it should not.
type Handlers struct {
	store *Store
	now   func() time.Time
}

// NewHandlers binds the transport to its store.
func NewHandlers(store *Store, now func() time.Time) Handlers {
	return Handlers{store: store, now: now}
}

// ListIntroRequests returns the asks about one contact that the caller is
// party to.
func (h Handlers) ListIntroRequests(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	rows, err := h.store.ForPerson(r.Context(), ids.UUID(id), listCap)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := make([]crmcontracts.IntroRequest, 0, len(rows))
	for i := range rows {
		out = append(out, wire(&rows[i]))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.IntroRequestListResponse{Data: out})
}

// CreateIntroRequest records one rep's ask that a colleague open a door.
func (h Handlers) CreateIntroRequest(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.IntroRequestInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	// A route through somebody names them and a direct one does not. Either
	// half alone describes a route nobody can act on, and the table refuses it
	// too — this is here so the caller is told which half is wrong.
	throughGiven := body.ThroughPersonId != nil
	wantsThrough := body.RouteType == crmcontracts.PersonGraphRouteTypeThroughContact
	if throughGiven != wantsThrough {
		httperr.Write(w, r, httperr.Validation(
			"through_person_id", "route_shape",
			"a route through a contact names them, and a direct route does not"))
		return
	}

	newID, err := h.store.Create(r.Context(), NewRequest{
		PersonID:        ids.UUID(id),
		IntroducerUser:  ids.UUID(body.IntroducerUserId),
		RouteType:       string(body.RouteType),
		ThroughPersonID: optionalID(body.ThroughPersonId),
		InternalReason:  body.InternalReason,
		ValueForTarget:  deref(body.ValueForTarget),
		ForwardableNote: deref(body.ForwardableNote),
		NoteGeneratedBy: noteOriginOf(body.NoteGeneratedBy),
		NoteAIGenerated: body.NoteAiGenerated != nil && *body.NoteAiGenerated,
		FallbackPolicy:  fallbackOf(body.FallbackPolicy),
		NameDropAllowed: body.NameDropAllowed != nil && *body.NameDropAllowed,
		DueAt:           h.now().Add(askWindow),
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeAsk(w, r, newID, http.StatusCreated)
}

// DecideIntroRequest records the colleague's answer.
func (h Handlers) DecideIntroRequest(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.IntroRequestDecisionInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.store.Decide(r.Context(), ids.UUID(id), Status(body.Decision),
		deref(body.Reason), optionalID(body.SuggestedUserId), body.Version); err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeAsk(w, r, ids.UUID(id), http.StatusOK)
}

// CompleteIntroRequest records the handshake, or the rep having used a lent
// name. Which one is read from the ask's own state, never from this body.
func (h Handlers) CompleteIntroRequest(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.IntroRequestCompleteInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.store.Complete(
		r.Context(), ids.UUID(id), optionalID(body.SourceActivityId), body.Version); err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeAsk(w, r, ids.UUID(id), http.StatusOK)
}

// CancelIntroRequest withdraws an ask the caller made.
func (h Handlers) CancelIntroRequest(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.IntroRequestCancelInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	if err := h.store.Cancel(
		r.Context(), ids.UUID(id), deref(body.Reason), body.Version); err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeAsk(w, r, ids.UUID(id), http.StatusOK)
}

// writeAsk reads the ask back and puts it on the wire.
//
// Every write returns the row it produced, including its new version: a client
// that has just accepted needs the version to complete, and making it re-fetch
// invites it to complete against the one it sent.
func (h Handlers) writeAsk(w http.ResponseWriter, r *http.Request, id ids.UUID, code int) {
	saved, err := h.store.Get(r.Context(), id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, code, wire(saved))
}

// wire puts one ask on the contract's shape.
func wire(r *Request) crmcontracts.IntroRequest {
	out := crmcontracts.IntroRequest{
		Id:               openapi_types.UUID(r.ID),
		PersonId:         openapi_types.UUID(r.PersonID),
		RequesterUserId:  openapi_types.UUID(r.RequesterUserID),
		IntroducerUserId: openapi_types.UUID(r.IntroducerUser),
		RouteType:        crmcontracts.PersonGraphRouteType(r.RouteType),
		InternalReason:   r.InternalReason,
		Status:           crmcontracts.IntroRequestStatus(r.Status),
		NameDropAllowed:  r.NameDropAllowed,
		FallbackPolicy:   crmcontracts.IntroFallbackPolicy(r.FallbackPolicy),
		NoteGeneratedBy:  crmcontracts.IntroNoteOrigin(r.NoteGeneratedBy),
		NoteAiGenerated:  r.NoteAIGenerated,
		RequestedAt:      r.RequestedAt,
		DueAt:            r.DueAt,
		Version:          r.Version,
		DecidedAt:        r.DecidedAt,
		IntroducedAt:     r.IntroducedAt,
		NameDroppedAt:    r.NameDroppedAt,
		RepliedAt:        r.RepliedAt,
	}
	if r.ValueForTarget != "" {
		out.ValueForTarget = &r.ValueForTarget
	}
	if r.ForwardableNote != "" {
		out.ForwardableNote = &r.ForwardableNote
	}
	out.ThroughPersonId = wireID(r.ThroughPersonID)
	out.SuggestedUserId = wireID(r.SuggestedUserID)
	out.SourceActivityId = wireID(r.SourceActivityID)
	out.DecisionReason = r.DecisionReason
	return out
}

func wireID(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	u := openapi_types.UUID(*id)
	return &u
}

func optionalID(id *openapi_types.UUID) *ids.UUID {
	if id == nil {
		return nil
	}
	u := ids.UUID(*id)
	return &u
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// noteOriginOf defaults an unstated origin to `human`. A client that sends no
// provenance has a person typing, and claiming a model wrote it would mark
// honest copy as machine-authored.
func noteOriginOf(o *crmcontracts.IntroNoteOrigin) string {
	if o == nil {
		return "human"
	}
	return string(*o)
}

func fallbackOf(p *crmcontracts.IntroFallbackPolicy) string {
	if p == nil {
		return "none"
	}
	return string(*p)
}
