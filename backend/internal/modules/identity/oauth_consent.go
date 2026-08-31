// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What the consent screen renders, and the client it renders it for.

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// liveClient resolves client_id to the name a consent screen may show. An
// unknown, disabled, or soft-deleted client all read as apperrors.ErrNotFound
// — the same answer for all three, because which one it is would tell a
// prober something an admin's off switch is trying to hide. A genuine
// lookup failure (not "no such live client") is returned as itself, so a
// database problem is never mistaken for a client that does not exist.
func (s *Service) liveClient(ctx context.Context, clientID string) (string, error) {
	var name string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT c.client_name FROM oauth_client c WHERE c.client_id = $1 AND `+liveClientPredicate,
			clientID).Scan(&name)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return name, nil
}

// offlineRequested is all the consent screen takes from the client's scope
// parameter: whether it asked to stay connected without asking again. The access
// scopes in it are not read, because they decide nothing the screen renders — a
// lend grants the chosen passport's own scopes — while offline_access is about
// the connection's lifetime, which the human is approving and so must see.
//
// Unlike parseOAuthScopes this never errors: an unknown scope has already been
// refused on the authorize request this screen is rendering.
func offlineRequested(raw string) bool {
	return slices.Contains(strings.Fields(raw), scopeOfflineAccess)
}

// consentRequestPayload maps the read model onto the generated wire shape. The
// scope list is the closed vocabulary itself (passportScopeVocabulary), not a
// per-human computation: scopes are not a cap on their own — auth.Admit gates
// each tool and the granting human's RBAC is re-derived per call — so offering
// fewer here would narrow the screen without narrowing the authority, and the
// human would be choosing from a list that does not mean what it shows.
func consentRequestPayload(clientName string, offline bool) crmcontracts.ConsentRequest {
	scopes := make([]crmcontracts.ConsentRequestScopes, 0, len(passportScopeVocabulary))
	for _, scope := range passportScopeVocabulary {
		scopes = append(scopes, crmcontracts.ConsentRequestScopes(scope))
	}
	return crmcontracts.ConsentRequest{
		ClientName: clientName,
		Offline:    offline,
		Scopes:     scopes,
	}
}

// GetConsentRequest implements GET /oauth/consent-request. Human-only: an
// agent must never read or drive a consent screen (the generated agent
// policy enforces this from the contract's x-agent-access: human-only, but
// the check here is what a session-authenticated human actually needs).
func (h Handlers) GetConsentRequest(w http.ResponseWriter, r *http.Request, params crmcontracts.GetConsentRequestParams) {
	_, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "the consent screen belongs to the signed-in human whose authority the agent will borrow")
		return
	}
	clientName, err := h.svc.liveClient(r.Context(), params.ClientId)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The consent nonce is deliberately NOT read here. The cookie that carries it
	// is Path=/oauth/authorize, so a browser never sends it to this endpoint; the
	// redirect hands the nonce to the screen in the fragment instead, and the POST
	// still proves possession of the cookie. An endpoint that read it would 404
	// every real browser while a test setting the header by hand passed.
	httperr.WriteJSON(w, http.StatusOK,
		consentRequestPayload(clientName, offlineRequested(params.Scope)))
}
