// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What a consent COMMITS: the scopes the human ticked, the single-use code the
// client redeems, and the audit row naming the code — one transaction, so a
// code the audit trail cannot explain is a state this flow cannot reach.
//
// The ticks race nothing. The session that ticked them can end mid-consent, so
// the commit waits out any revocation of the member's credentials and re-reads
// that session first (holdConsentAgainstRevocation).
//
// The grant and passport are written at TOKEN EXCHANGE (oauth_token.go), not
// here. This function writes ONLY the authorization code row and its audit
// record. The code is the courier the client posts at /token; the grant and
// passport follow from the code's own data, read and validated at redemption.

import (
	"context"
	"errors"
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

// consentChoice is what the human posted: the scopes ticked and the areas chosen.
type consentChoice struct {
	scopes string
	areas  principal.WorkAreas
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
	ctx context.Context, id Identity, sessionHash string, choice consentChoice, req authorizeRequest,
) (code string, err error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return "", err
	}
	scopes, err := parseConsentedScopes(choice.scopes)
	if err != nil {
		return "", err
	}
	areas := choice.areas
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
		if err := holdConsentAgainstRevocation(ctx, tx, req.ClientID, id.UserID, sessionHash); err != nil {
			return err
		}
		if err := reapDeadCredentials(ctx, tx, "oauth_authorization_code"); err != nil {
			return err
		}
		var codeID ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO oauth_authorization_code
			  (code_hash, client_id, user_id, scopes, code_challenge, redirect_uri, resource, expires_at, work_areas)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), now() + $8::interval, $9)
			RETURNING id`,
			codeHash, req.ClientID, id.UserID, storedScopes, req.CodeChallenge,
			req.RedirectURI, req.Resource, authCodeTTL.String(), areasColumn(areas)).Scan(&codeID); err != nil {
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
				auditFieldAreas:          areasColumn(areas),
			})
		return err
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

// errConsentingSessionEnded refuses a consent whose session ended before its
// code could commit, so no code outlives the credentials a revocation ended.
var errConsentingSessionEnded = errors.New("oauth: the consenting session has ended")

// credentialAuthorityLock is the advisory key a consent shares and
// endCredentialAuthority takes alone. A row lock on app_user would invert
// redemption's auth_token → app_user order (tokenlockorder_test.go).
const credentialAuthorityLock = `hashtextextended('credential_authority:' || $1::text, 0)`

// holdConsentAgainstRevocation orders this consent against endCredentialAuthority.
// A revocation that went first has already revoked the session read below. One
// that waits here sees the code after it commits. The session read is its own
// statement: under `READ COMMITTED` only a fresh snapshot sees the revocation
// waited on. The client and member rows are taken before the advisory lock, the
// order issueGrant and a deactivation already hold them in.
func holdConsentAgainstRevocation(ctx context.Context, tx pgx.Tx, clientID string, userID ids.UserID, sessionHash string) error {
	if _, err := tx.Exec(ctx,
		`SELECT 1 FROM oauth_client WHERE client_id = $1 FOR KEY SHARE`, clientID); err != nil {
		return err
	}
	var held ids.UserID
	err := tx.QueryRow(ctx,
		`SELECT id FROM app_user WHERE id = $1 AND `+LiveMemberSQL("")+` FOR KEY SHARE`, userID).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return errConsentingSessionEnded
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(`+credentialAuthorityLock+`)`, userID.String()); err != nil {
		return err
	}
	var live bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM session
		 WHERE user_id = $1 AND token_hash = $2 AND revoked_at IS NULL
		   AND now() < idle_expires_at AND now() < expires_at)`,
		userID, sessionHash).Scan(&live); err != nil {
		return err
	}
	if !live {
		return errConsentingSessionEnded
	}
	return nil
}
