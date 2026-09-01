// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the notices transport: settle one, and raise one for a teammate.
// There is no listing — a notice's content reaches its reader on the Worklist's
// notices lane, and a person reads only their own.
type Handlers struct {
	store *Store
	mates Teammates
}

// NewHandlers binds the transport to its store and to the membership question
// coaching is gated on.
func NewHandlers(store *Store, mates Teammates) Handlers {
	return Handlers{store: store, mates: mates}
}

// RaiseNotice records one person's coaching nudge to a teammate.
func (h Handlers) RaiseNotice(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.RaiseNoticeRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	var note string
	if req.Note != nil {
		note = *req.Note
	}
	notice, err := h.store.RaiseCoachNotice(
		r.Context(), h.mates, ids.From[ids.UserKind](ids.UUID(req.RecipientUserId)), req.Kind, note)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, noticeWire(notice))
}

// noticeWire is the raised notice as the contract returns it. An empty body is
// ABSENT rather than an empty string: the coach added no note, which is a
// different fact from adding a blank one.
func noticeWire(n Notice) crmcontracts.Notice {
	out := crmcontracts.Notice{
		Id:        openapi_types.UUID(n.ID),
		Kind:      crmcontracts.NoticeKind(n.Kind),
		Subject:   n.Subject,
		CreatedAt: n.CreatedAt,
	}
	if n.Body != "" {
		body := n.Body
		out.Body = &body
	}
	return out
}

// MarkNoticeRead settles one notice for the acting person.
func (h Handlers) MarkNoticeRead(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if err := h.store.MarkRead(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
