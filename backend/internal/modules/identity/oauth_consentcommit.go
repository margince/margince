// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What a consent COMMITS: the scopes the human ticked, the single-use code the
// client redeems, and the audit row naming the grant — one transaction, so a
// code the audit trail cannot explain is a state this flow cannot reach.
//
// There is no re-check and no row lock here, and that is the whole difference
// from what this file replaced. A consent used to borrow its authority from a
// second row (a passport the human "lent"), which could be revoked in another
// tab while the screen sat open — so the commit had to re-resolve that row
// under a lock. The human's own ticks are not a row, cannot go stale, and race
// nothing.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
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

// mintConsentedAuthorizationCode writes a consent, the grant and the authorization code
// the client will redeem, all in one transaction. The code is single-use: its row
// is deleted on a successful redeem (oauth_token.go), and a second redeem on the
// same code finds no row, which is a refusal.
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
		// The GRANT: what the human authorized. Its id becomes the passport's
		// oauth_grant_id, which tells the Settings list it is a connection.
		grantID := ids.NewV7()
		if err := tx.QueryRow(ctx, `
			INSERT INTO oauth_grant (
				id, client_id, redirect_uri, scope, refresh_allowed,
				captured_by
			) VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id`,
			grantID, req.ClientID, req.RedirectURI, strings.Join(scopes, " "),
			req.Offline, id.UserID).Scan(&grantID); err != nil {
			return err
		}
		// The PASSPORT: the credential the client will mint from the code. It
		// carries the scopes the human actually ticked.
		passportID := ids.NewV7()
		if err := tx.QueryRow(ctx, `
			INSERT INTO passport (
				id, workspace_id, on_behalf_of, label, scopes, expires_at,
				oauth_grant_id, captured_by
			) VALUES ($1, $2, $3, $4, $5, now() + interval '1 hour', $6, $7)
			RETURNING id`,
			passportID, id.WorkspaceID, id.UserID, "", scopes, grantID,
			id.UserID).Scan(&passportID); err != nil {
			return err
		}
		// The AUTHORIZATION CODE: a single-use password the client posts at the
		// token endpoint. The code hash is stored, not the plaintext: the
		// plaintext is the courier and appears in no durable record.
		codeHash := hashOAuthCode(code)
		if _, err := tx.Exec(ctx, `
			INSERT INTO oauth_authorization_code (
				code_hash, grant_id, client_id, scopes, code_challenge,
				redirect_uri, resource, expires_at, captured_by
			) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), now() + $8::interval, $9)`,
			codeHash, grantID, req.ClientID, storedScopes, req.CodeChallenge,
			req.RedirectURI, req.Resource, authCodeTTL.String(), id.UserID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return code, nil
}
