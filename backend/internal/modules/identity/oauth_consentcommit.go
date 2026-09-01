// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What a consent COMMITS: the scopes the human ticked, the single-use code the
// client redeems, and the audit row naming the code — one transaction, so a
// code the audit trail cannot explain is a state this flow cannot reach.
//
// There is no re-check and no row lock here, and that is the whole difference
// from what this file replaced. A consent used to borrow its authority from a
// second row (a passport the human "lent"), which could be revoked in another
// tab while the screen sat open — so the commit had to re-resolve that row
// under a lock. The human's own ticks are not a row, cannot go stale, and race
// nothing.
//
// The grant and passport are written at TOKEN EXCHANGE (oauth_token.go), not
// here. This function writes ONLY the authorization code row and its audit
// record. The code is the courier the client posts at /token; the grant and
// passport follow from the code's own data, read and validated at redemption.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// parseConsentedScopes reads the human's ticks off the consent form and answers
// with them in vocabulary order.
//
// Every refusal here is TERMINAL, which is a change in kind from the lend it
// replaces. A lent passport could legitimately die between render and submit,
// so its refusal was recoverable and the screen asked again. A scope outside
// the vocabulary — or none at all — cannot come from a screen this server
// rendered: it is a hand-built form or a bug, and there is nothing for the
// human to choose again.
//
// The order is imposed rather than preserved. Form order is the order the
// checkboxes were rendered in, which no reader of the audit row will know, and
// passportScopeVocabulary is already the one ascending-authority list both
// discovery documents derive from.
func parseConsentedScopes(raw string) ([]string, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil, fmt.Errorf("a consent grants at least one scope: %w", apperrors.ErrInvalidArgument)
	}
	ticked := make(map[principal.Scope]bool, len(fields))
	for _, field := range fields {
		scope := principal.Scope(field)
		if !validScopes[scope] {
			return nil, fmt.Errorf("%q is not a grantable scope: %w", field, apperrors.ErrInvalidArgument)
		}
		if ticked[scope] {
			return nil, fmt.Errorf("%q is ticked twice: %w", field, apperrors.ErrInvalidArgument)
		}
		ticked[scope] = true
	}
	out := make([]string, 0, len(ticked))
	for _, scope := range passportScopeVocabulary {
		if ticked[scope] {
			out = append(out, string(scope))
		}
	}
	return out, nil
}

// mintConsentedAuthorizationCode writes the single-use code the client will
// redeem, and the audit row naming the code — all in ONE transaction, so a
// code the audit trail cannot explain is a state this flow cannot reach.
//
// The code is the ONLY row written here. The grant and passport follow from
// the code's data (scopes, client_id, user_id) at token redemption
// (oauth_token.go), so writing them here would be duplicate. This function
// answers with the plaintext courier; only the courier's hash is stored.
func (s *Service) mintConsentedAuthorizationCode(
	ctx context.Context, id Identity, rawScopes string, req authorizeRequest,
) (code string, err error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return "", err
	}
	scopes, err := parseConsentedScopes(rawScopes)
	if err != nil {
		return "", err
	}
	code, err = randomToken()
	if err != nil {
		return "", err
	}
	storedScopes := scopes
	if req.Offline {
		storedScopes = append(append([]string{}, scopes...), scopeOfflineAccess)
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The AUTHORIZATION CODE: a single-use password the client posts at the
		// token endpoint. The code row is the contract: if the code does not name
		// the scopes, client, and user, the redemption has no authority to mint
		// from (oauth_token.go deletes the code on a successful redeem).
		codeHash := hashOAuthCode(code)
		var codeID ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO oauth_authorization_code
			  (code_hash, client_id, user_id, scopes, code_challenge, redirect_uri, resource, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), now() + $8::interval)
			RETURNING id`,
			codeHash, req.ClientID, id.UserID, storedScopes, req.CodeChallenge,
			req.RedirectURI, req.Resource, authCodeTTL.String()).Scan(&codeID); err != nil {
			return err
		}
		// Audit the consent: what scopes the human ticked, for what client, with
		// what refresh authority. The grant will be minted at redemption and
		// record the same pair, so the consent and its redemption read as one
		// story.
		_, err := storekit.Audit(ctx, tx, "create", "oauth_authorization_code", codeID, nil,
			map[string]any{
				auditFieldClientID:       req.ClientID,
				auditFieldScopes:         scopes,
				auditFieldRefreshAllowed: req.Offline,
			})
		return err
	})
	if err != nil {
		return "", err
	}
	return code, nil
}
