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
	// Token is the plaintext, and `json:"-"` is not decoration: this struct
	// carries a bearer credential over one person's record, and every field of
	// it is one `WriteJSON(w, issued)` away from a response body. The tag makes
	// that line publish nothing rather than the whole credential, which is the
	// defect the operator-held double-opt-in endpoint was retired for.
	//
	// It is the belt, not the braces. Nothing may hand this token to a sink at
	// all, which is what TestThePlaintextConfirmTokenReachesNoSinkButTheMail
	// (backend/gates/doitokenexposure_test.go) holds — and that gate is what
	// catches a log line, which a json tag cannot.
	Token     string `json:"-"`
	ExpiresAt time.Time
	// DeliveredTo is where the link was posted. Returned rather than taken, so
	// the mailbox the consent claim rests on is the subject's own.
	DeliveredTo string
	// NoticeCasesDischarged is how many disclosure duties this mail settled.
	//
	// Zero is the ordinary case: most people are owed nothing, and a case
	// already discharged inside the cooldown is deliberately not moved again.
	NoticeCasesDischarged int
	// Staged reports whether the mail was queued on the durable lane.
	//
	// FALSE is a real outcome and not an error: an installation with no relay
	// configured still mints the token — refusing would invite a retry that
	// mints a second link and silently supersedes the first — and the screen
	// tells an operator to configure one.
	Staged bool
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
	return s.issueLink(ctx, personID, LinkRecordConfirmation, ids.PurposeID{}, "")
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
// expectedAddress is the address the REQUESTER named, and it is checked against
// the address the link will actually reach rather than replacing it. Pass ""
// when the caller named a person rather than typing an address.
//
// A caller who could CHOOSE the destination could choose a stranger's, so the
// mint keeps deriving it from the person's own record. But a typed address
// resolves to whatever person already holds it, including on a NON-primary
// address, and the link then goes to that person's primary instead. That is how
// a subscription requested from an address somebody stopped using arrives at
// the one they still read. Refusing the mismatch is the only honest answer: the
// two addresses disagree about who asked, and the mint cannot tell which is
// right.
func (s *Store) IssueConsentLink(ctx context.Context, personID ids.PersonID, purposeID ids.PurposeID, expectedAddress string) (IssuedConfirm, error) {
	if err := httperr.RequireBodyID(purposeIDField, purposeID.UUID); err != nil {
		return IssuedConfirm{}, err
	}
	return s.issueLink(ctx, personID, LinkConsentConfirmation, purposeID, expectedAddress)
}

// deliveryAddressTx reads the subject's own live primary address, the same way
// the confirm card reads it.
//
// A person carrying none has no mailbox to prove, so there is nothing the link
// could evidence: it is refused rather than minted against an address nobody
// holds.
func deliveryAddressTx(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (string, error) {
	var deliveredTo string
	err := tx.QueryRow(ctx, `
		SELECT email FROM person_email
		 WHERE person_id = $1 AND archived_at IS NULL
		 ORDER BY is_primary DESC, created_at
		 LIMIT 1`, personID).Scan(&deliveredTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", &ValidationError{
			Field:  personIDKey,
			Reason: "this contact carries no live email address, so there is no mailbox a confirm link could reach",
		}
	}
	return deliveredTo, err
}

// requireConfirmablePurposeTx refuses a link minted against a purpose this mail
// cannot honestly ask about. A record-confirmation link names no purpose and is
// exempt.
//
// THREE answers, because they fail differently for the reader.
//
// A purpose that RESOLVES TO NOTHING — never existed, or belongs to a workspace
// this caller cannot see — is not found. It is deliberately not a 422 naming
// the field: every other required body id on this surface answers 404 for an id
// that names no visible row, so that a caller cannot tell "no such purpose"
// from "not yours" and read the difference as an enumeration. Claiming such an
// id was ARCHIVED is also simply untrue, and it sends an operator looking for a
// purpose to unarchive that nobody ever created.
//
// An ARCHIVED purpose exists and is refused with its own sentence: a link
// minted against it would mail and then dead-end, because consentCardFor
// resolves only a live purpose, so the subject opens a 404 sent in the
// installation's own name.
//
// A purpose that does not REQUIRE double opt-in is a different mistake — the
// mail would ask somebody to confirm a subscription whose grant never needed
// confirming, and the answer would be recorded as mailbox-proven evidence
// nobody asked for. The endpoint's whole subject is the double-opt-in purpose.
func requireConfirmablePurposeTx(ctx context.Context, tx pgx.Tx, purposeID ids.PurposeID) error {
	if purposeID.UUID == (ids.UUID{}) {
		return nil
	}
	var requiresDOI, archived bool
	// Read WITHOUT the live filter, so absence and archival stay separable. The
	// filtered read answered no-rows for both and had to guess which; it always
	// guessed archived.
	err := tx.QueryRow(ctx,
		`SELECT requires_double_opt_in, archived_at IS NOT NULL FROM consent_purpose
		  WHERE id = $1`, purposeID).Scan(&requiresDOI, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if archived {
		return &ValidationError{
			Field:  purposeIDField,
			Reason: "this purpose is archived, so a link asking somebody to confirm it could not be answered",
		}
	}
	if !requiresDOI {
		return &ValidationError{
			Field:  purposeIDField,
			Reason: "this purpose is not confirmed by double opt-in, so there is nothing for a mailed link to ask",
		}
	}
	return nil
}

func (s *Store) issueLink(ctx context.Context, personID ids.PersonID, kind string, purposeID ids.PurposeID, expectedAddress string) (IssuedConfirm, error) {
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return IssuedConfirm{}, err
	}
	token, err := newConfirmToken()
	if err != nil {
		return IssuedConfirm{}, err
	}
	var out IssuedConfirm
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		deliveredTo, err := admitLinkTx(ctx, tx, linkRequest{
			personID:        personID,
			purposeID:       purposeID,
			expectedAddress: expectedAddress,
		})
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
			personID, hashPublicToken(token), deliveredTo, issued, expires,
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
		// The mail itself, on THIS transaction. The token row and the message
		// that carries it commit together or not at all: a token minted without
		// its mail is a link nobody was ever sent, and a mail staged without its
		// token is a link that resolves to nothing.
		staged, err := s.stageConfirmMail(ctx, tx, confirmMailInput{
			personID:   personID,
			recipient:  deliveredTo,
			kind:       kind,
			tokenRowID: tokenRowID,
			link:       s.confirmLink(token),
			expiresAt:  expires,
		})
		if err != nil {
			return err
		}
		out.Staged = staged
		// The mail that discharges a duty is what moves the duty. A
		// record-confirmation link IS the Art. 14 disclosure route named in
		// allowed_routes, so sending one settles the cases that named it —
		// on this transaction, so a rolled-back mail leaves no duty marked
		// handled by a message nobody sent.
		//
		// GATED ON `staged`, which is not the same guarantee as the
		// transaction. An installation with no lane wired still mints the link
		// and reports queued=false — a supported outcome, not an error, so it
		// COMMITS. Discharging there would write an audit row saying a duty was
		// met by a message that was never staged, and the cooldown would then
		// suppress the genuine send once an operator fixed the relay.
		if route, discharges := noticeRouteFor(kind); discharges && staged {
			moved, err := dischargeNoticeCases(ctx, tx, personID, route, issued)
			if err != nil {
				return err
			}
			out.NoticeCasesDischarged = moved
		}
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
			hashPublicToken(token), s.now().UTC()).Scan(
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
		hashPublicToken(token), s.now().UTC()).Scan(&personID, &kind)
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
		hashPublicToken(token), s.now().UTC()).Scan(
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

// hashPublicToken hashes any credential a public link carries — a confirm
// token and a withdrawal credential both. Named for the surface rather than
// for the first caller, because the second one arrived and a shared helper
// called hashConfirmToken would have read as the wrong function being reused.
func hashPublicToken(token string) string {
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
