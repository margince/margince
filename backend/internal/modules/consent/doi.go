// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// doiTokenTTL bounds the confirmation window: an unclicked confirmation
// mail is a refusal, not a standing credential.
const doiTokenTTL = 72 * time.Hour

// IssuedDOI carries the plaintext exactly once, with the redemption
// deadline the caller (and the data subject's mail) may show.
type IssuedDOI struct {
	Token     string
	ExpiresAt time.Time
}

// purposeIDField names the wire path every purpose refusal on this surface
// points at. Named once because three refusals use it, and a field slot holds a
// wire field path, never prose.
const purposeIDField = "purpose_id"

// IssueDoubleOptIn mints the single-use confirmation token a DOI grant
// must later present. Only the sha256 lands in the database — the
// session/passport secret discipline — so a stolen table cannot confirm
// anything. A fresh issuance supersedes any unredeemed prior token for
// the same (person, purpose): supersession is expiry, so the redeem
// path needs no extra state. Delivery of the plaintext to the data
// subject is the deployment's mail seam (the BookMeeting-invite
// stance); the deliver flag is recorded on the audit row so the
// issuance intent stays attributable, and the plaintext never lands in
// audit or outbox payloads.
func (s *Store) IssueDoubleOptIn(ctx context.Context, personID ids.PersonID, purposeID ids.PurposeID, deliver bool) (IssuedDOI, error) {
	// A token confirms consent FOR a purpose; without one there is nothing to
	// confirm. Unguarded, the zero UUID reaches the purpose read and answers
	// not-found for a purpose nobody named.
	if err := httperr.RequireBodyID(purposeIDField, purposeID.UUID); err != nil {
		return IssuedDOI{}, err
	}
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return IssuedDOI{}, err
	}
	token, err := newDOIToken()
	if err != nil {
		return IssuedDOI{}, err
	}
	var out IssuedDOI
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Live, for the same reason Record refuses a grant against an archived
		// subject: a double opt-in is a GRANT flow — the token exists so the
		// subject can confirm one — so issuing it for a person the installation
		// was told to forget is the same lawful-basis claim by a slower route.
		//
		// The `archived_at IS NULL` on the purpose read below is a different
		// question and does not cover this one: it asks whether the PURPOSE is
		// live, not whether the subject is.
		if err := auth.EnsureWritableLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		var requiresDOI bool
		err := tx.QueryRow(ctx,
			`SELECT requires_double_opt_in FROM consent_purpose WHERE id = $1 AND archived_at IS NULL`,
			purposeID).Scan(&requiresDOI)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("purpose %s: %w", purposeID, apperrors.ErrNotFound)
		}
		if err != nil {
			return err
		}
		if !requiresDOI {
			return &ValidationError{Field: purposeIDField, Reason: "purpose does not require a double opt-in"}
		}
		issued := s.now().UTC()
		if _, err := tx.Exec(ctx, `
			UPDATE consent_doi_token SET expires_at = $3
			WHERE person_id = $1 AND purpose_id = $2 AND consumed_at IS NULL AND expires_at > $3`,
			personID, purposeID, issued); err != nil {
			return err
		}
		// consent_doi_token rows are a security artifact, not a kernel
		// entity, so the row id stays untyped.
		var tokenRowID ids.UUID
		// The subject's liveness rides the INSERT as well as the probe above,
		// and it is worth being exact about what that buys. It closes the
		// window between the probe and this statement — an erasure committing
		// there would otherwise mint a live invitation to grant consent for
		// somebody the installation has just been told to forget, and the token
		// would work. It does NOT close the window between this statement and
		// COMMIT; an erasure landing there still wins, and closing that needs a
		// row lock on the subject rather than a predicate.
		//
		// So this is a narrowing, not a proof, and it is the only statement in
		// the module that carries one — every other EnsureWritableLive site is
		// probe-then-write and takes the same residual risk unremarked. Both
		// halves are tracked on #2574, which decides lock-versus-predicate for
		// the primitive rather than leaving each statement to answer it.
		//
		// No row means the subject went while we were deciding, which is the
		// answer the probe would have given a moment earlier, so it refuses the
		// same way rather than failing on a scan.
		err = tx.QueryRow(ctx, `
			INSERT INTO consent_doi_token (person_id, purpose_id, token_hash, issued_at, expires_at)
			SELECT $1, $2, $3, $4, $5
			 WHERE EXISTS (SELECT 1 FROM person WHERE id = $1 AND archived_at IS NULL)
			RETURNING id`,
			personID, purposeID, hashDOIToken(token), issued, issued.Add(doiTokenTTL)).Scan(&tokenRowID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "create", "consent_doi_token", tokenRowID, nil, map[string]any{
			"person_id":  personID,
			"purpose_id": purposeID,
			"expires_at": issued.Add(doiTokenTTL),
			"deliver":    deliver,
		}); err != nil {
			return err
		}
		out = IssuedDOI{Token: token, ExpiresAt: issued.Add(doiTokenTTL)}
		return nil
	})
	if err != nil {
		return IssuedDOI{}, err
	}
	return out, nil
}

// consumeDOIToken redeems the round-trip proof exactly once. The grant
// is only as strong as the token the confirmation mail carried, so a
// value that was never issued, was already used, belongs to another
// person×purpose, or has expired refuses the grant instead of recording
// a half-true confirmation.
func (s *Store) consumeDOIToken(ctx context.Context, tx pgx.Tx, personID ids.PersonID, purposeID ids.PurposeID, token string) (time.Time, error) {
	confirmed := s.now().UTC()
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		UPDATE consent_doi_token SET consumed_at = $1
		WHERE person_id = $2 AND purpose_id = $3 AND token_hash = $4
		  AND consumed_at IS NULL AND expires_at > $1
		RETURNING id`,
		confirmed, personID, purposeID, hashDOIToken(token)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, &ValidationError{Field: "double_opt_in_token", Reason: "not a currently issued double opt-in token"}
	}
	if err != nil {
		return time.Time{}, err
	}
	return confirmed, nil
}

func newDOIToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("consent: doi token entropy: %w", err)
	}
	return "doi_" + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func hashDOIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
