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
	minted, err := newWithdrawalToken()
	if err != nil {
		return "", err
	}
	// ON CONFLICT DO NOTHING against the live index, then read back: the mint
	// races itself whenever two sends to one address are prepared at once, and
	// the loser must return the winner's credential rather than an error. The
	// RETURNING arm tells the two apart — a row comes back only when this
	// statement inserted it, which is exactly when the token is disclosable.
	var inserted bool
	err = tx.QueryRow(ctx, `
		INSERT INTO withdrawal_credential
		    (token_hash, address, person_id, lead_id, scope, purpose_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
		ON CONFLICT DO NOTHING
		RETURNING true`,
		hashPublicToken(minted), address,
		zeroAsNull(in.PersonID.UUID), zeroAsNull(in.LeadID.UUID),
		in.Scope, zeroAsNull(in.PurposeID),
		withdrawalCredentialLife.String()).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		// A live credential already covers this address and scope. Its token is
		// unrecoverable by construction, and the link carrying it still works.
		return "", nil
	}
	if err != nil {
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

// RevokeSubjectCredentials kills every live withdrawal credential a subject
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

// WithdrawalTokenForEmail mints (or reuses) the withdrawal credential this
// message's one-click link will carry.
//
// IT RESOLVES A LEAD TOO, which is the second half of the defect this file
// closes. PreferenceTokenForEmail resolves persons only, so a lead-only
// recipient's marketing mail goes out with no List-Unsubscribe header at all —
// the send path treats "no token" as "no unsubscribe surface" and carries none.
//
// An address matching NOBODY still gets a credential. The message is going to
// that address regardless, and an opt-out is about where the mail went; a
// recipient we cannot name is exactly the one least able to ask a human.
//
// ok is false only when a credential already covers this address and scope, in
// which case the previous link still works and there is nothing new to write
// into the mail. The caller keeps the header it would otherwise have built.
func (s *Store) WithdrawalTokenForEmail(
	ctx context.Context, recipientEmail, purposeKey string,
) (token string, ok bool, err error) {
	address := normalizeAddress(recipientEmail)
	if address == "" {
		return "", false, nil
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		in := WithdrawalMintInput{Address: address, Scope: WithdrawalScopeAllMarketing}
		if err := bindWithdrawalSubject(ctx, tx, &in); err != nil {
			return err
		}
		if purposeKey != "" {
			var purposeID ids.UUID
			switch scanErr := tx.QueryRow(ctx,
				`SELECT id FROM consent_purpose WHERE key = $1`, purposeKey).Scan(&purposeID); {
			case errors.Is(scanErr, pgx.ErrNoRows):
				// A key no purpose carries: the all-marketing scope is the
				// honest fallback, and it is the wider of the two, so the link
				// never stops less than the recipient expects.
			case scanErr != nil:
				return fmt.Errorf("consent: resolving the purpose for the unsubscribe link: %w", scanErr)
			default:
				in.Scope, in.PurposeID = WithdrawalScopeNamedPurpose, purposeID
			}
		}
		var mintErr error
		token, mintErr = s.EnsureWithdrawalCredentialTx(ctx, tx, in)
		return mintErr
	})
	if err != nil {
		return "", false, err
	}
	return token, token != "", nil
}

// bindWithdrawalSubject attaches the record holding this address, when one
// does. A person wins over a lead: a promoted lead's mail is the person's.
//
// AMBIGUITY LEAVES THE CREDENTIAL UNBOUND rather than picking. Two live
// records on one address is a data problem, and choosing between them by row
// order would stamp one person's id on the other's opt-out link. The credential
// still works — it names the address, which is what the withdrawal acts on.
func bindWithdrawalSubject(ctx context.Context, tx pgx.Tx, in *WithdrawalMintInput) error {
	// count(*) rather than a LIMIT and a Scan: a bare read of the first row is
	// the silent pick this function exists not to make, and it looks identical
	// to the unambiguous case at the call site.
	person, err := theOneSubjectHolding(ctx, tx, `
		SELECT pe.person_id
		  FROM person_email pe
		  JOIN person p ON p.id = pe.person_id AND p.archived_at IS NULL
		 WHERE lower(pe.email) = $1 AND pe.archived_at IS NULL`, in.Address)
	if err != nil {
		return fmt.Errorf("consent: resolving the person the unsubscribe link is for: %w", err)
	}
	if !person.IsZero() {
		in.PersonID = ids.From[ids.PersonKind](person)
		return nil
	}
	lead, err := theOneSubjectHolding(ctx, tx, `
		SELECT id FROM lead
		 WHERE lower(email) = $1 AND archived_at IS NULL AND merged_into_id IS NULL`, in.Address)
	if err != nil {
		return fmt.Errorf("consent: resolving the lead the unsubscribe link is for: %w", err)
	}
	if !lead.IsZero() {
		in.LeadID = ids.From[ids.LeadKind](lead)
	}
	return nil
}

// theOneSubjectHolding returns the single record this query matches, or the
// zero id when it matches none OR SEVERAL.
//
// Several is deliberately the same answer as none. Stamping one of two
// candidates onto the credential would put one person's id on the other's
// opt-out link, and the link works either way: it names the address, which is
// what the withdrawal acts on.
func theOneSubjectHolding(ctx context.Context, tx pgx.Tx, query, address string) (ids.UUID, error) {
	rows, err := tx.Query(ctx, query+" LIMIT 2", address)
	if err != nil {
		return ids.UUID{}, err
	}
	matches, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return ids.UUID{}, err
	}
	if len(matches) != 1 {
		return ids.UUID{}, nil
	}
	return matches[0], nil
}
