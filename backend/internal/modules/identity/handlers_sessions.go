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

// ListMySessions implements GET /me/sessions. Always the caller's own: the
// service takes the user id off the bound principal, and the session behind THIS
// request's cookie is the one the list marks current.
func (h Handlers) ListMySessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.svc.ListSessions(r.Context(), currentSessionHash(r))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, mySessionList(sessions))
}

// RevokeMySession implements DELETE /me/sessions/{sessionId}. The service scopes
// the revoke to the caller's own user id, taken off the principal, and answers a
// stranger's session id with not-found — so this handler adds no ownership check
// of its own, which would be a second copy of the gate that matters, free to
// drift from it.
func (h Handlers) RevokeMySession(w http.ResponseWriter, r *http.Request, sessionID openapi_types.UUID) {
	if err := h.svc.RevokeSession(r.Context(), ids.UUID(sessionID)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// currentSessionHash reduces the request's session cookie to the value a session
// row stores, so the list can name which row is this very request's. No cookie —
// a state the admission gate makes unreachable on this route — hashes to nothing
// and simply leaves no session marked current.
func currentSessionHash(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return hashToken(cookie.Value)
}

func mySessionList(sessions []MySession) crmcontracts.MySessionList {
	out := crmcontracts.MySessionList{Sessions: make([]crmcontracts.MySession, 0, len(sessions))}
	for _, s := range sessions {
		out.Sessions = append(out.Sessions, crmcontracts.MySession{
			Id:           openapi_types.UUID(s.ID),
			UserAgent:    s.UserAgent,
			SignedInAt:   s.SignedInAt,
			LastActiveAt: s.LastActiveAt,
			Current:      s.Current,
		})
	}
	return out
}
