// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListUserSessions implements GET /users/{id}/sessions — a user_admin holder's
// view of another member's live sessions. The service carries the object gate
// and the not-found for an out-of-scope target, so this handler only resolves
// the actor and the id.
func (h Handlers) ListUserSessions(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	sessions, err := h.svc.ListUserSessions(r.Context(), actor, ids.UserID{UUID: ids.UUID(id)})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, userSessionList(sessions))
}

// RevokeUserSession implements DELETE /users/{id}/sessions/{sessionId} — a
// user_admin holder ending one of another member's sessions.
func (h Handlers) RevokeUserSession(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, sessionID openapi_types.UUID) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	if err := h.svc.RevokeUserSession(r.Context(), actor, ids.UserID{UUID: ids.UUID(id)}, ids.UUID(sessionID)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func userSessionList(sessions []UserSession) crmcontracts.UserSessionList {
	out := crmcontracts.UserSessionList{Sessions: make([]crmcontracts.UserSession, 0, len(sessions))}
	for _, s := range sessions {
		out.Sessions = append(out.Sessions, crmcontracts.UserSession{
			Id:           openapi_types.UUID(s.ID),
			UserAgent:    s.UserAgent,
			SignedInAt:   s.SignedInAt,
			LastActiveAt: s.LastActiveAt,
		})
	}
	return out
}
