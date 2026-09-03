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
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// personIDKey names the subject in an audit payload, and the wire path a
// refusal about them points at. A typo in either would cost a reader the row
// they were looking for, which is why it is a constant rather than a literal.
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
	// DeliveredTo is where the send path must post it. Returned rather than
	// taken, so the mailbox the consent claim rests on is the subject's own.
	DeliveredTo string
}

// ConfirmRef is a token's resolution: whose record it opens, the address the
// link went to, and the token row itself — which a submission cites as the
// capability it arrived through.
type ConfirmRef struct {
	PersonID    ids.PersonID
	TokenID     ids.UUID
	DeliveredTo string
	// Kind says which question the spent link asked, and PurposeID names the
	// marketing purpose when it asked about one. The submit branches on these:
	// a consent link records an answer for its OWN purpose and accepts nothing
	// else, because it was mailed asking one question and a link that could
	// also edit the record would be a wider capability than the mail described.
	Kind      string
	PurposeID ids.PurposeID
}

// IssueConfirmToken mints the single-use link for one person and returns the
// address it must be delivered to. Only the sha256 lands in the database, so a
// stolen table opens nobody's record.
//
// The address is DERIVED here rather than accepted from the caller, and that is
// the security property rather than a convenience. A grant made through this
// link completes with no confirmation mail, on the claim that the link reached
// the subject's own mailbox — so a caller who could name the address could name
// somebody else's, hand out the plaintext, and produce a consent that looks
// defensible against a mailbox the subject never held. The retired double-opt-in issuance was
// structurally immune for the same reason: it takes no address at all.
//
// A fresh issuance supersedes any unspent prior token for the same person:
// supersession is expiry, exactly as the double-opt-in path does it, so the
// resolve path needs no extra state. Delivery of the plaintext is the caller's,
// which is what keeps this store free of a mail dependency.
func (s *Store) IssueConfirmToken(ctx context.Context, personID ids.PersonID) (IssuedConfirm, error) {
	return s.issueLink(ctx, personID, LinkRecordConfirmation, ids.PurposeID{})
}

// IssueConsentLink mints the link a double-opt-in purpose is confirmed by.
//
// It is the SAME mechanism as the record-confirmation link beside it, and that
// is the whole design: the server picks the address off the person's own
// record, only the hash is stored, the plaintext is mailed and never returned,
// and spending the link is what proves the mailbox. The retired double-opt-in
// endpoint had none of those properties — it handed the plaintext to an
// operator, so one person could complete both halves of a round trip whose only
// value is that the subject completed it.
func (s *Store) IssueConsentLink(ctx context.Context, personID ids.PersonID, purposeID ids.PurposeID) (IssuedConfirm, error) {
	if err := httperr.RequireBodyID(purposeIDField, purposeID.UUID); err != nil {
		return IssuedConfirm{}, err
	}
	return s.issueLink(ctx, personID, LinkConsentConfirmation, purposeID)
}

func (s *Store) issueLink(ctx context.Context, personID ids.PersonID, kind string, purposeID ids.PurposeID) (IssuedConfirm, error) {
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
		// same ordering the erasure path takes and for the same reason. What
		// this mints is a working link to one person's record; an erasure
		// committing after an unheld probe would leave the installation posting
		// it to somebody it had just been told to forget.
		if err := auth.HoldWritableLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		// The subject's own live primary address, read the same way the card
		// reads it. A person carrying none has no mailbox to prove, so there is
		// nothing this link could evidence and it is refused rather than minted
		// against an address nobody holds.
		var deliveredTo string
		err := tx.QueryRow(ctx, `
			SELECT email FROM person_email
			 WHERE person_id = $1 AND archived_at IS NULL
			 ORDER BY is_primary DESC, created_at
			 LIMIT 1`, personID).Scan(&deliveredTo)
		if errors.Is(err, pgx.ErrNoRows) {
			return &ValidationError{
				Field:  personIDKey,
				Reason: "this contact carries no live email address, so there is no mailbox a confirm link could reach",
			}
		}
		if err != nil {
			return err
		}
		issued := s.now().UTC()
		expires := issued.Add(confirmTokenTTL)
		// Per KIND, and per purpose within a kind. A fresh record-confirmation
		// link must not expire somebody's pending consent link and the other
		// way round: they ask different questions and arrive in different
		// mails, so superseding across them would silently kill an answer the
		// subject was still coming back to.
		if _, err := tx.Exec(ctx, `
			UPDATE confirm_token SET expires_at = $2
			WHERE person_id = $1 AND consumed_at IS NULL AND expires_at > $2
			  AND kind = $3 AND purpose_id IS NOT DISTINCT FROM $4`,
			personID, issued, kind, nullablePurpose(purposeID)); err != nil {
			return err
		}
		// A confirm_token row is a security artifact, not a kernel entity, so
		// the row id stays untyped — as consent_doi_token's does.
		var tokenRowID ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO confirm_token (person_id, token_hash, delivered_to, issued_at, expires_at, kind, purpose_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			personID, hashConfirmToken(token), deliveredTo, issued, expires,
			kind, nullablePurpose(purposeID)).Scan(&tokenRowID); err != nil {
			return err
		}
		// The address is audited because it is the evidence: a later reader
		// asking why a grant counted needs to see which mailbox was reached.
		// The plaintext token never lands in audit or outbox payloads.
		if _, err := storekit.Audit(ctx, tx, "create", "confirm_token", tokenRowID, nil, map[string]any{
			personIDKey:    personID,
			"delivered_to": deliveredTo,
			"expires_at":   expires,
			auditKeyKind:   kind,
		}); err != nil {
			return err
		}
		out = IssuedConfirm{Token: token, ExpiresAt: expires, DeliveredTo: deliveredTo}
		return nil
	})
	if err != nil {
		return IssuedConfirm{}, err
	}
	return out, nil
}

// ResolveConfirmToken answers whose record a confirm link opens. Unknown,
// expired, already-spent and belonging-to-an-archived-subject read as absent,
// all four identically, so the surface never becomes an oracle for which it was.
//
// The liveness test is here rather than in the card read, so both verbs get it
// from one statement. An ordinary archive does not delete these rows — only
// Art. 17 erasure and the retention anonymizer do — so a rep archiving a contact
// who holds a live link would otherwise leave the next click answering 500,
// and a submit would burn the link before refusing.
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
	// The kind is read HERE and not only at the spend, because the read is a
	// disclosure of its own: a consent link's page must show the subscription
	// question and not the person's record card. Gating the write and leaving
	// the read open would hand whoever holds a consent link everything the
	// record page shows, which is wider than the mail that carried it.
	var purposeID *ids.PurposeID
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			UPDATE confirm_token ct SET opened_at = coalesce(ct.opened_at, $2)
			WHERE ct.token_hash = $1 AND ct.consumed_at IS NULL AND ct.expires_at > $2
			  AND EXISTS (SELECT 1 FROM person p
			               WHERE p.id = ct.person_id AND p.archived_at IS NULL)
			RETURNING ct.person_id, ct.id, ct.delivered_to, ct.kind, ct.purpose_id`,
			hashConfirmToken(token), s.now().UTC()).Scan(
			&ref.PersonID, &ref.TokenID, &ref.DeliveredTo, &ref.Kind, &purposeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if purposeID != nil {
			ref.PurposeID = *purposeID
		}
		return err
	})
	if err != nil {
		return ConfirmRef{}, err
	}
	return ref, nil
}

// subjectOfConfirmTokenTx names whose link this is, without taking a row lock.
//
// A plain read, and that is what it is for: the submit has to know the subject
// BEFORE it locks anything, because the subject row is the first lock its
// transaction may take. Art. 17 erasure holds the person and then deletes these
// token rows, so a transaction touching the token first would close a cycle.
//
// Naming the subject is not authorization. The spend below is what redeems the
// link, and it runs under the subject lock this read makes possible.
func (s *Store) subjectOfConfirmTokenTx(ctx context.Context, tx pgx.Tx, token string) (ids.PersonID, string, error) {
	var personID ids.PersonID
	var kind string
	// The kind comes back with the subject so a submission wider than the mail
	// can be refused BEFORE the link is spent. Refusing after would burn the
	// subject's one chance to answer on a request the store was never going to
	// stand behind — the same reason validateConfirmSubmission runs first.
	err := tx.QueryRow(ctx, `
		SELECT ct.person_id, ct.kind FROM confirm_token ct
		 WHERE ct.token_hash = $1 AND ct.consumed_at IS NULL AND ct.expires_at > $2
		   AND EXISTS (SELECT 1 FROM person p
		                WHERE p.id = ct.person_id AND p.archived_at IS NULL)`,
		hashConfirmToken(token), s.now().UTC()).Scan(&personID, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.PersonID{}, "", fmt.Errorf("confirm token: %w", apperrors.ErrNotFound)
	}
	return personID, kind, err
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
	var purposeID *ids.PurposeID
	err := tx.QueryRow(ctx, `
		UPDATE confirm_token ct SET consumed_at = $2
		WHERE ct.token_hash = $1 AND ct.consumed_at IS NULL AND ct.expires_at > $2
		  AND EXISTS (SELECT 1 FROM person p
		               WHERE p.id = ct.person_id AND p.archived_at IS NULL)
		RETURNING ct.person_id, ct.id, ct.delivered_to, ct.kind, ct.purpose_id`,
		hashConfirmToken(token), s.now().UTC()).Scan(
		&ref.PersonID, &ref.TokenID, &ref.DeliveredTo, &ref.Kind, &purposeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmRef{}, fmt.Errorf("confirm token: %w", apperrors.ErrNotFound)
	}
	if purposeID != nil {
		ref.PurposeID = *purposeID
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

// auditKeyKind names which question a minted link asks, on its audit row. One
// spelling, because three call sites write it.
const auditKeyKind = "kind"

// The two questions a mailed link can carry.
const (
	// LinkRecordConfirmation asks the subject to check what is held about them.
	LinkRecordConfirmation = "record_confirmation"
	// LinkConsentConfirmation asks them to confirm one marketing purpose. This
	// is the double opt-in, and spending the link is the only thing that
	// completes one.
	LinkConsentConfirmation = "consent_confirmation"
)

// nullablePurpose keeps the column NULL for a record link, which is what the
// row's own CHECK demands: a consent link names its purpose and a record link
// never does.
func nullablePurpose(id ids.PurposeID) *ids.PurposeID {
	if id.UUID == (ids.UUID{}) {
		return nil
	}
	return &id
}
