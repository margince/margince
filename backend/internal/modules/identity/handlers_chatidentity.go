// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// chatHintDigits is how many trailing characters of an account the list shows.
const chatHintDigits = 4

// chatHint hides all but the last characters of an account.
func chatHint(account string) string {
	if len(account) <= chatHintDigits {
		return "…" + account
	}
	return "…" + account[len(account)-chatHintDigits:]
}

// ListMyChatIdentities implements GET /me/chat-identities.
func (h Handlers) ListMyChatIdentities(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.MyChatIdentities(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.ChatIdentityList{Data: make([]crmcontracts.ChatIdentity, 0, len(rows))}
	for _, row := range rows {
		item := crmcontracts.ChatIdentity{
			Network: crmcontracts.ChatNetwork(row.Network), VerifiedAt: row.VerifiedAt, ExpiresAt: row.CodeExpiresAt,
		}
		if row.Account != "" {
			hint := chatHint(row.Account)
			item.AccountHint = &hint
		}
		out.Data = append(out.Data, item)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// StartMyChatVerification implements POST /me/chat-identities/{network}.
func (h Handlers) StartMyChatVerification(w http.ResponseWriter, r *http.Request, network string) {
	code, expires, err := h.svc.StartChatVerification(r.Context(), ChatNetwork(network))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ChatVerificationStarted{Network: crmcontracts.ChatNetwork(network), Code: code, ExpiresAt: expires})
}

// RemoveMyChatIdentity implements DELETE /me/chat-identities/{network}.
func (h Handlers) RemoveMyChatIdentity(w http.ResponseWriter, r *http.Request, network string) {
	if err := h.svc.RemoveChatIdentity(r.Context(), ChatNetwork(network)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
