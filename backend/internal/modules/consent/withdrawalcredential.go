// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The credential that only ever withdraws.
//
// preference_token is one credential doing two jobs: it opens the preference
// centre, which READS the recipient's per-purpose state and can GRANT, and it
// is also what the RFC 8058 one-click POST carries. Those two want opposite
// lifetimes. Read authority over somebody's consent record should be short,
// because the link sits in a mailbox forever. A withdrawal should last as long
// as the mail does, because a person unsubscribing from a two-year-old
// newsletter is exercising a right that does not expire.
//
// Today the short lifetime wins for both — 30 days, sliding, revoked on
// rotation — so pressing unsubscribe on older mail answers "this link is no
// longer valid" and the only way left to stop the mail is to ask a human. That
// is exactly the friction one-click unsubscribe exists to remove.
//
// So the authority is split. This credential CANNOT read a consent state and
// CANNOT grant; it names a scope it may never exceed, and that is what makes it
// safe to keep for years.
//
// HASHED, unlike preference_token, which stores its value in plaintext. This
// one outlives every rotation and sits in mail archives and proxy logs, so a
// database read must not hand somebody a working withdrawal link for every
// recipient we have ever mailed.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// withdrawalCredentialLife is the ceiling, not a sliding window.
//
// The link is meant to outlive the mail that carried it, and 24 months is what
// this installation retains the mail itself for. A credential outliving every
// copy of the message protects nobody: there is no longer a link anywhere for
// a person to press, only a working credential for anyone who finds one.
const withdrawalCredentialLife = 24 * 30 * 24 * time.Hour

// The two things a withdrawal link may be for. Neither reaches business
// correspondence: an invoice is not a subscription anybody opted into, so it
// is not a thing to unsubscribe from, and a link that stopped it would be
// answering a question the recipient did not ask.
const (
	// WithdrawalScopeNamedPurpose stops the one subscription the link was
	// minted for, which is what the List-Unsubscribe header on a newsletter
	// means.
	WithdrawalScopeNamedPurpose = "named_purpose"
	// WithdrawalScopeAllMarketing performs the same sweep the preference
	// centre's "stop all marketing" does, for a link minted without a purpose.
	WithdrawalScopeAllMarketing = "all_marketing"
)

// The reasons a credential dies. They are not interchangeable: the page has to
// answer an erased subject differently from a rotated one, and telling somebody
// "that address had a subscription" after an erasure would disclose the very
// thing the erasure removed.
const (
	WithdrawalRevokedErasure           = "erasure"
	WithdrawalRevokedCompromise        = "compromise"
	WithdrawalRevokedMergedPredecessor = "merged_predecessor"
	WithdrawalRevokedSuperseded        = "superseded"
)

// fieldKeyPurpose names the contract field a refusal points at. Three refusals
// in this package share it, so a change to the contract's field name is a
// change here rather than a hunt through string literals.
const fieldKeyPurpose = "purpose"

// withdrawalTokenPrefix is how ResolvePublicToken tells the two families apart
// without a database read. A prefix rather than a length or a probe, so a
// caller reading a log can see which authority a link carried.
const withdrawalTokenPrefix = "wd_"

// WithdrawalRef is what a withdrawal link speaks for: an address, the record
// holding it if one still does, and the scope the link may not exceed.
//
// The ADDRESS is the identity here, not the person. A subject may be merged,
// archived or erased between the send and the press, and the opt-out is about
// where the mail went.
type WithdrawalRef struct {
	CredentialID ids.UUID
	Address      string
	PersonID     ids.PersonID
	LeadID       ids.LeadID
	Scope        string
	PurposeID    ids.UUID
	// Legacy is true when this ref came from a preference_token rather than a
	// withdrawal credential — an old link, minted before this table existed,
	// being honoured for its withdrawal half only.
	Legacy bool
}

// newWithdrawalToken mints the value that goes in the mail. Same entropy and
// encoding as the confirm token beside it; only the prefix differs.
func newWithdrawalToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("consent: withdrawal token entropy: %w", err)
	}
	return withdrawalTokenPrefix + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

// EnsureWithdrawalCredentialTx returns the live credential for this address and
// scope, minting one if there is none.
//
// IDEMPOTENT ON THE LIVE ROW, which is what the unique index enforces: re-sending
// a newsletter reuses the link the last mail carried rather than minting a second
// credential that works just as well. Two live links for one subscription is two
// bearer credentials to leak, and revoking one would leave the other working.
//
// It returns the TOKEN only when it minted one. A credential that already exists
// cannot produce its token again — the table holds a hash — which is the point:
// a database read must not yield working links. A send that needs a URL for an
// existing credential is a send whose previous link still works.
func (s *Store) EnsureWithdrawalCredentialTx(
	ctx context.Context, tx pgx.Tx, in WithdrawalMintInput,
) (token string, err error) {
	// GATED, unlike the resolve below. This MINTS a bearer credential that can
	// stop somebody's mail for two years, so the caller has to be entitled to
	// act on that person — the same grant IssueConfirmToken asks for, and for
	// the same reason: a credential minted for a person is authority over them.
	//
	// person:update rather than person:read, because minting one changes what
	// can be done to the record even though it writes no consent state.
	if err := auth.Require(ctx, entityPerson, principal.ActionUpdate); err != nil {
		return "", err
	}
	address := normalizeAddress(in.Address)
	if address == "" {
		return "", &ValidationError{
			Field:  "address",
			Reason: "a withdrawal link is written to an address, and this one is empty",
		}
	}
	if in.Scope != WithdrawalScopeNamedPurpose && in.Scope != WithdrawalScopeAllMarketing {
		return "", &ValidationError{
			Field:  "scope",
			Reason: "a withdrawal link stops one named subscription or all marketing; there is no third scope",
		}
	}
	// THE SUBJECT MUST STILL BE LIVE AT THIS POINT IN THIS TRANSACTION.
	//
	// Statements in a read-committed transaction each take a fresh snapshot,
	// so an erasure committing between the caller's address lookup and this
	// insert would leave the mint writing a NEW capability — carrying the
	// plaintext address — for the subject whose credentials that erasure just
	// deleted. The person row survives an anonymize-in-place, so the foreign
	// key does not catch it and the fresh token resolves happily.
	//
	// EnsureWritableLive, not the VISIBLE twin PreferenceTokenForEmail uses.
	// Person is a shareable table, so a manual `read` share widens visibility
	// without widening write authority — and this path WRITES a capability
	// over the subject's mail. A caller holding only a read share must not
	// mint one. Live rather than plain, for the reason the preference mint
	// gives: a plain probe answers "still there" for an anonymized tombstone,
	// which is precisely the row the erasure race leaves behind.
	if !in.PersonID.IsZero() {
		if err := auth.EnsureWritableLive(ctx, tx, entityPerson, in.PersonID.UUID); err != nil {
			return "", err
		}
		// AND HELD until this transaction commits. EnsureWritableLive reads a
		// snapshot; the insert below is a later statement, so an erasure
		// committing between the two would find no credential to delete and
		// this would then write one — restoring both a working capability and
		// the plaintext address for a subject the installation just certified
		// erased. Holding the row makes the erasure queue behind this mint,
		// and its delete then sees the row this wrote.
		if err := auth.LockSubjectLive(ctx, tx, entityPerson, in.PersonID.UUID); err != nil {
			return "", err
		}
	}
	minted, err := newWithdrawalToken()
	if err != nil {
		return "", err
	}
	// EVERY SEND MINTS ITS OWN, so every message a recipient holds carries a
	// link that works. The first spelling had a unique index over the live rows
	// and tried to reuse — which the hash makes impossible, since the token is
	// unrecoverable — so it returned nothing and the send shipped no header.
	// The second revoked the old credential and minted fresh, which killed the
	// link in the mail the recipient already had. Both traded away the thing
	// this table exists to provide. See the migration for the full argument.
	if _, err := tx.Exec(ctx, `
		INSERT INTO withdrawal_credential
		    (token_hash, address, person_id, lead_id, scope, purpose_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)`,
		hashPublicToken(minted), address,
		zeroAsNull(in.PersonID.UUID), zeroAsNull(in.LeadID.UUID),
		in.Scope, zeroAsNull(in.PurposeID),
		withdrawalCredentialLife.String()); err != nil {
		return "", fmt.Errorf("consent: minting the withdrawal credential: %w", err)
	}
	return minted, nil
}

// WithdrawalMintInput names what a link is for.
type WithdrawalMintInput struct {
	Address   string
	PersonID  ids.PersonID
	LeadID    ids.LeadID
	Scope     string
	PurposeID ids.UUID
}

// ResolveWithdrawalToken answers which address and scope a withdrawal link
// speaks for, accepting BOTH families.
//
// A token with no wd_ prefix is tried as a legacy preference token, because
// every link already in somebody's mailbox carries one of those and pressing it
// must still work. See legacyPreferenceTokenAsWithdrawal for what that adapter
// will and will not honour.
//
// Unknown, expired and revoked all read as absent, identically, so the surface
// never becomes an oracle for which of the three it was.
func (s *Store) ResolveWithdrawalToken(ctx context.Context, token string) (WithdrawalRef, error) {
	var ref WithdrawalRef
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		var err error
		ref, err = resolveWithdrawalTokenTx(ctx, tx, token)
		return err
	})
	return ref, err
}

func resolveWithdrawalTokenTx(ctx context.Context, tx pgx.Tx, token string) (WithdrawalRef, error) {
	if !strings.HasPrefix(token, withdrawalTokenPrefix) {
		return legacyPreferenceTokenAsWithdrawal(ctx, tx, token)
	}
	var (
		ref       WithdrawalRef
		personID  *ids.UUID
		leadID    *ids.UUID
		purposeID *ids.UUID
	)
	err := tx.QueryRow(ctx, `
		SELECT id, address, person_id, lead_id, scope, purpose_id
		  FROM withdrawal_credential
		 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		hashPublicToken(token)).Scan(
		&ref.CredentialID, &ref.Address, &personID, &leadID, &ref.Scope, &purposeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WithdrawalRef{}, apperrors.ErrNotFound
	}
	if err != nil {
		return WithdrawalRef{}, fmt.Errorf("consent: resolving the withdrawal link: %w", err)
	}
	if personID != nil {
		ref.PersonID = ids.From[ids.PersonKind](*personID)
	}
	if leadID != nil {
		ref.LeadID = ids.From[ids.LeadKind](*leadID)
	}
	if purposeID != nil {
		ref.PurposeID = *purposeID
	}
	return ref, nil
}

// legacyPreferenceTokenAsWithdrawal honours the withdrawal half of a link
// minted before this table existed.
//
// IGNORING expires_at, which is the entire point. Those tokens slide 30 days
// and are revoked on every rotation, so an unexpired one is the exception among
// links already sitting in mailboxes. Refusing them would mean the change that
// fixed expiring withdrawal links shipped with every existing withdrawal link
// still expiring.
//
// REFUSING three revocation reasons, because those are not rotations. An
// erasure removed the subject; a compromised token is in somebody else's hands;
// a merged predecessor's credential belongs to a record that no longer receives
// mail. Honouring any of the three would let a link act for a subject who is
// gone or for a holder who is not the recipient. A rotated or expired token is
// neither — it is the same recipient's own older link.
//
// It grants NOTHING beyond withdrawal: the ref carries an address and the
// all-marketing scope, and the caller cannot read a consent state with it.
func legacyPreferenceTokenAsWithdrawal(ctx context.Context, tx pgx.Tx, token string) (WithdrawalRef, error) {
	var (
		personID ids.UUID
		address  *string
	)
	err := tx.QueryRow(ctx, `
		SELECT pt.person_id, pe.email
		  FROM preference_token pt
		  LEFT JOIN person_email pe ON pe.id = pt.person_email_id
		 WHERE pt.token = $1
		   AND (pt.revoked_reason IS NULL OR pt.revoked_reason IN ('rotated', 'expired'))`,
		token).Scan(&personID, &address)
	if errors.Is(err, pgx.ErrNoRows) {
		return WithdrawalRef{}, apperrors.ErrNotFound
	}
	if err != nil {
		return WithdrawalRef{}, fmt.Errorf("consent: resolving the legacy withdrawal link: %w", err)
	}
	ref := WithdrawalRef{
		PersonID: ids.From[ids.PersonKind](personID),
		// ALL MARKETING, never a named purpose. A preference token names no
		// subscription — it opened a page listing all of them — so the only
		// honest reading of a press on one is "stop the marketing".
		Scope:  WithdrawalScopeAllMarketing,
		Legacy: true,
	}
	if address != nil {
		ref.Address = normalizeAddress(*address)
	}
	return ref, nil
}

// RevokeSubjectCredentialsTx kills every live withdrawal credential a subject
// holds, for a stated reason.
//
// Reached from erasure (the subject is gone), from a merge (the predecessor no
// longer receives mail) and from the admin compromise route. Each writes a
// different reason because the public page answers each differently.
func (s *Store) RevokeSubjectCredentialsTx(
	ctx context.Context, tx pgx.Tx, personID ids.PersonID, reason string,
) error {
	// GATED for the mint's reason inverted: revoking a credential takes away a
	// person's working opt-out link, which is a change to what they can do
	// about their own mail.
	if err := auth.Require(ctx, entityPerson, principal.ActionUpdate); err != nil {
		return err
	}
	switch reason {
	case WithdrawalRevokedErasure, WithdrawalRevokedCompromise,
		WithdrawalRevokedMergedPredecessor, WithdrawalRevokedSuperseded:
	default:
		return &ValidationError{
			Field:  "reason",
			Reason: "a revocation states one of the four reasons the public page can answer",
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE withdrawal_credential
		   SET revoked_at = now(), revoked_reason = $2
		 WHERE person_id = $1 AND revoked_at IS NULL`, personID, reason); err != nil {
		return fmt.Errorf("consent: revoking the subject's withdrawal links: %w", err)
	}
	return nil
}

// normalizeAddress lowercases and trims, matching the lower(address) the live
// index is built on so a mint and a lookup agree about which row they mean.
//
// authorizecap.go and preference.go each normalize addresses for their own
// purposes too. Consolidating them would change the cap's lock key, which does
// not belong in a slice about withdrawal links.
func normalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// purposeKeyByID reads the key a named-purpose credential was minted for, so
// the withdrawal names the same purpose the message was sent under.
//
// Unexported and unauthorized on purpose: it is reached only from the public
// one-click path, which has already resolved a credential naming this exact
// purpose id. The caller is not choosing the id, it is reading back the one
// stamped on the credential it holds.
func (s *Store) purposeKeyByID(ctx context.Context, purposeID ids.UUID) (string, error) {
	var key string
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT key FROM consent_purpose WHERE id = $1`, purposeID).Scan(&key)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("consent: reading the purpose the link was minted for: %w", err)
	}
	return key, nil
}
