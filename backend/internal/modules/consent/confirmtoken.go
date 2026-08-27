// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The capability a contact is emailed so they can see what is held about them,
// correct it, and answer the marketing question.
//
// It is a sibling of consent_doi_token rather than of preference_token, and the
// difference is what each one shows. A preference link shows a list of switches
// and must keep working for as long as mail can reach the inbox, so it is
// plaintext, reusable and long-lived. This one shows the person's own record and
// can complete a marketing consent, so it is hashed at rest, short-lived, and
// spent on first submit.
//
// The delivery address travels ON the row because a consent granted here rests
// on it: the click stands in for a double-opt-in round trip only because the
// link reached the subject's own mailbox.

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
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// personIDKey is the audit-payload key naming the subject. A typo would make
// that row unsearchable by the very field a later reader looks it up on, which
// is why it is a constant rather than a literal at each payload.
const personIDKey = "person_id"

// confirmTokenTTL bounds how long a link showing somebody their own record
// stays live. Longer than the 72-hour double-opt-in window, because a person may
// read the mail next week and the page is a courtesy rather than a deadline;
// short enough that an old mailbox stops being a window onto a live record.
const confirmTokenTTL = 14 * 24 * time.Hour

// IssuedConfirm carries the plaintext exactly once, with the deadline the mail
// may show the recipient.
type IssuedConfirm struct {
	Token     string
	ExpiresAt time.Time
}

// ConfirmRef is a token's resolution: whose record it opens, the address the
// link went to, and the token row itself — which a submission cites as the
// capability it arrived through.
type ConfirmRef struct {
	PersonID    ids.PersonID
	TokenID     ids.UUID
	DeliveredTo string
}

// IssueConfirmToken mints the single-use link for one person, recording the
// address it is to be delivered to. Only the sha256 lands in the database, so a
// stolen table opens nobody's record.
//
// A fresh issuance supersedes any unspent prior token for the same person:
// supersession is expiry, exactly as the double-opt-in path does it, so the
// resolve path needs no extra state. Delivery of the plaintext is the caller's,
// which is what keeps this store free of a mail dependency.
func (s *Store) IssueConfirmToken(ctx context.Context, personID ids.PersonID, deliveredTo string) (IssuedConfirm, error) {
	if deliveredTo == "" {
		return IssuedConfirm{}, &ValidationError{
			Field:  "delivered_to",
			Reason: "a confirm link is evidence of reaching one mailbox, so the address it is sent to is required",
		}
	}
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return IssuedConfirm{}, err
	}
	token, err := newConfirmToken()
	if err != nil {
		return IssuedConfirm{}, err
	}
	var out IssuedConfirm
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Live, and HELD before this transaction takes any other row lock — the
		// same ordering IssueDoubleOptIn takes and for the same reason. What
		// this mints is a working link to one person's record; an erasure
		// committing after an unheld probe would leave the installation posting
		// it to somebody it had just been told to forget.
		if err := auth.HoldWritableLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		issued := s.now().UTC()
		expires := issued.Add(confirmTokenTTL)
		if _, err := tx.Exec(ctx, `
			UPDATE confirm_token SET expires_at = $2
			WHERE person_id = $1 AND consumed_at IS NULL AND expires_at > $2`,
			personID, issued); err != nil {
			return err
		}
		// A confirm_token row is a security artifact, not a kernel entity, so
		// the row id stays untyped — as consent_doi_token's does.
		var tokenRowID ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO confirm_token (person_id, token_hash, delivered_to, issued_at, expires_at)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id`,
			personID, hashConfirmToken(token), deliveredTo, issued, expires).Scan(&tokenRowID); err != nil {
			return err
		}
		// The address is audited because it is the evidence: a later reader
		// asking why a grant counted needs to see which mailbox was reached.
		// The plaintext token never lands in audit or outbox payloads.
		if _, err := storekit.Audit(ctx, tx, "create", "confirm_token", tokenRowID, nil, map[string]any{
			personIDKey:    personID,
			"delivered_to": deliveredTo,
			"expires_at":   expires,
		}); err != nil {
			return err
		}
		out = IssuedConfirm{Token: token, ExpiresAt: expires}
		return nil
	})
	if err != nil {
		return IssuedConfirm{}, err
	}
	return out, nil
}

// ResolveConfirmToken answers whose record a confirm link opens. Unknown,
// expired and already-spent read as absent, all three identically, so the
// surface never becomes an oracle for which of the three it was.
//
// Resolution runs outside row-level security for the same reason the preference
// resolver does: the surface it serves has no session, and the token IS the
// authorization.
//
// It stamps opened_at on first resolution, which is the ask-to-click chain a
// later reader follows from the token row: the mail went out at issued_at, the
// person opened it at opened_at, and the answer landed at consumed_at.
func (s *Store) ResolveConfirmToken(ctx context.Context, token string) (ConfirmRef, error) {
	var ref ConfirmRef
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			UPDATE confirm_token SET opened_at = coalesce(opened_at, $2)
			WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
			RETURNING person_id, id, delivered_to`,
			hashConfirmToken(token), s.now().UTC()).Scan(&ref.PersonID, &ref.TokenID, &ref.DeliveredTo)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	})
	if err != nil {
		return ConfirmRef{}, err
	}
	return ref, nil
}

// spendConfirmTokenTx marks the link used, inside the caller's transaction so
// the submit it authorizes and the spending of it commit together. A token that
// is no longer live refuses rather than being spent twice, which is what makes a
// replayed submit a refusal instead of a second write.
//
// This is also what stops a MailboxProof from being a claim anyone can make:
// the proof is only reachable through a token this statement could spend.
func (s *Store) spendConfirmTokenTx(ctx context.Context, tx pgx.Tx, token string) (ConfirmRef, error) {
	var ref ConfirmRef
	err := tx.QueryRow(ctx, `
		UPDATE confirm_token SET consumed_at = $2
		WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING person_id, id, delivered_to`,
		hashConfirmToken(token), s.now().UTC()).Scan(&ref.PersonID, &ref.TokenID, &ref.DeliveredTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmRef{}, fmt.Errorf("confirm token: %w", apperrors.ErrNotFound)
	}
	return ref, err
}

func newConfirmToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("consent: confirm token entropy: %w", err)
	}
	return "cfm_" + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

func hashConfirmToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
