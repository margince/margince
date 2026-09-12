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
// as the mail does, because a contact unsubscribing from a two-year-old
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
// a contact to press, only a working credential for anyone who finds one.
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
// The ADDRESS is the identity here, not the contact. A subject may be merged,
// archived or erased between the send and the press, and the opt-out is about
// where the mail went.
type WithdrawalRef struct {
	CredentialID ids.UUID
	Address      string
	ContactID    ids.ContactID
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

// EnsureWithdrawalCredentialTx mints a credential for a subject the CALLER
// NAMES, and takes write authority over that subject for doing so.
//
// contact:update and the WRITABLE probe, because naming somebody's id and asking
// for a bearer token over their mail IS a claim of authority over that record.
// `contact` is shareable, so a manual `read` grant widens who can SEE a contact
// without widening who may act on them, and a read-share holder must not mint
// one. The send door below is the one that asks a different question.
func (s *Store) EnsureWithdrawalCredentialTx(
	ctx context.Context, tx pgx.Tx, in WithdrawalMintInput,
) (token string, err error) {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return "", err
	}
	if err := validWithdrawalMint(in); err != nil {
		return "", err
	}
	if !in.ContactID.IsZero() {
		if err := auth.EnsureWritableLive(ctx, tx, entityContact, in.ContactID.UUID); err != nil {
			return "", err
		}
		if err := auth.LockSubjectLive(ctx, tx, entityContact, in.ContactID.UUID); err != nil {
			return "", err
		}
	}
	return insertWithdrawalCredential(ctx, tx, in)
}

// ensureWithdrawalCredentialForSendTx mints the credential a MESSAGE carries,
// for a subject resolved from the address that message is going to.
//
// contact:READ and the VISIBLE probe, and the difference from the door above is
// the question rather than the row. This caller names nobody: bindWithdrawalSubject
// resolves the subject from the address, the send is already authorized against
// that recipient by the consent gate and by the activity it creates, and the
// mail is going there whether or not this succeeds. So the strict probe protects
// nobody — it refused every sender without contact:update, an agent holding
// `activity:create` + `contact:read` among them, and the message then shipped
// with no working List-Unsubscribe URL, which is the failure this table exists
// to end.
//
// The authority is also strictly less than the SIBLING mint on the same path
// needs: PreferenceTokenForEmail takes contact:read plus this same visible probe
// and yields a credential that reads a consent state, withdraws AND grants,
// where this one can only stop mail.
//
// Unexported, so the weaker probe cannot be reached by naming a subject: the
// only way in is the send path in this package.
func (s *Store) ensureWithdrawalCredentialForSendTx(
	ctx context.Context, tx pgx.Tx, in WithdrawalMintInput,
) (token string, err error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return "", err
	}
	if err := validWithdrawalMint(in); err != nil {
		return "", err
	}
	if !in.ContactID.IsZero() {
		if err := auth.EnsureVisibleLive(ctx, tx, entityContact, in.ContactID.UUID); err != nil {
			return "", err
		}
		if err := auth.LockSubjectLive(ctx, tx, entityContact, in.ContactID.UUID); err != nil {
			return "", err
		}
	}
	return insertWithdrawalCredential(ctx, tx, in)
}

// validWithdrawalMint refuses what no link can be written for, before either
// door's probe spends a query on it.
func validWithdrawalMint(in WithdrawalMintInput) error {
	if normalizeAddress(in.Address) == "" {
		return &ValidationError{
			Field:  "address",
			Reason: "a withdrawal link is written to an address, and this one is empty",
		}
	}
	if in.Scope != WithdrawalScopeNamedPurpose && in.Scope != WithdrawalScopeAllMarketing {
		return &ValidationError{
			Field:  "scope",
			Reason: "a withdrawal link stops one named subscription or all marketing; there is no third scope",
		}
	}
	return nil
}

// insertWithdrawalCredential writes the row both doors mint, after whichever
// probe that door took.
//
// ONE PER SEND, and it always returns the token it minted. The table holds a
// hash, so an existing credential cannot produce its token again — which is the
// point, since a database read must not yield working links — and that is why
// reuse is impossible rather than merely unwanted. Several live rows per address
// is therefore the correct shape: each is a bearer token for stopping that
// recipient's mail and nothing else, exactly like the messages carrying them,
// and revocation is by SUBJECT so no row is left working behind.
//
// The subject is HELD by the caller's LockSubjectLive until this commits: the
// probe above it reads a snapshot, and this insert is a later statement, so an
// erasure committing between the two would find no credential to delete and
// this would then write one — restoring a working capability and the plaintext
// address for a subject the installation just certified erased.
func insertWithdrawalCredential(ctx context.Context, tx pgx.Tx, in WithdrawalMintInput) (string, error) {
	minted, err := newWithdrawalToken()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO withdrawal_credential
		    (token_hash, address, contact_id, lead_id, scope, purpose_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)`,
		hashPublicToken(minted), normalizeAddress(in.Address),
		zeroAsNull(in.ContactID.UUID), zeroAsNull(in.LeadID.UUID),
		in.Scope, zeroAsNull(in.PurposeID),
		withdrawalCredentialLife.String()); err != nil {
		return "", fmt.Errorf("consent: minting the withdrawal credential: %w", err)
	}
	return minted, nil
}

// WithdrawalMintInput names what a link is for.
type WithdrawalMintInput struct {
	Address   string
	ContactID ids.ContactID
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
		contactID *ids.UUID
		leadID    *ids.UUID
		purposeID *ids.UUID
	)
	err := tx.QueryRow(ctx, `
		SELECT id, address, contact_id, lead_id, scope, purpose_id
		  FROM withdrawal_credential
		 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		hashPublicToken(token)).Scan(
		&ref.CredentialID, &ref.Address, &contactID, &leadID, &ref.Scope, &purposeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WithdrawalRef{}, apperrors.ErrNotFound
	}
	if err != nil {
		return WithdrawalRef{}, fmt.Errorf("consent: resolving the withdrawal link: %w", err)
	}
	if contactID != nil {
		ref.ContactID = ids.From[ids.ContactKind](*contactID)
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
		contactID ids.UUID
		address   *string
	)
	err := tx.QueryRow(ctx, `
		SELECT pt.contact_id, pe.email
		  FROM preference_token pt
		  LEFT JOIN contact_email pe ON pe.id = pt.contact_email_id
		 WHERE pt.token = $1
		   AND (pt.revoked_reason IS NULL OR pt.revoked_reason IN ('rotated', 'expired'))`,
		token).Scan(&contactID, &address)
	if errors.Is(err, pgx.ErrNoRows) {
		return WithdrawalRef{}, apperrors.ErrNotFound
	}
	if err != nil {
		return WithdrawalRef{}, fmt.Errorf("consent: resolving the legacy withdrawal link: %w", err)
	}
	ref := WithdrawalRef{
		ContactID: ids.From[ids.ContactKind](contactID),
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
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, reason string,
) error {
	// GATED for the mint's reason inverted: revoking a credential takes away a
	// contact's working opt-out link, which is a change to what they can do
	// about their own mail.
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
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
		 WHERE contact_id = $1 AND revoked_at IS NULL`, contactID, reason); err != nil {
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
