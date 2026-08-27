// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The buyer-facing preference center + RFC 8058 one-click unsubscribe
// (B-E11.32): the no-login surface over THIS module's consent engine. A
// recipient reaches it through an unguessable preference_token carried in
// the List-Unsubscribe URL; the token resolves to (workspace, person)
// before any session exists, and every choice rides the normal consent
// write shape (proof row + audit + consent.changed) with a distinct
// `preference_center` source. The token holder proved control of the
// mailbox by receiving the token, so — unlike the fully-anonymous booking
// form — an explicit re-grant is the data subject's own opt-in, not a
// consent hijack; a withdrawal always goes through.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PurposeTransactional is the one locked purpose: operational mail about a
// live deal has a lawful lane that a marketing opt-out must not silence
// (data-model §3.4 per-purpose separation; UC-E11-07 step 6). The
// preference center refuses to change it.
const PurposeTransactional = "transactional"

// PurposeMarketingEmail is the seeded marketing lane (seed.go), and the one the
// confirm page asks about. Named because two surfaces resolve it by key and a
// literal in either would drift from the seed without anything failing.
const PurposeMarketingEmail = "marketing_email"

// LockedPurpose reports whether a purpose may not be changed from the
// public preference surface. Locked purposes also carry no unsubscribe
// header — there is nothing to unsubscribe from.
func LockedPurpose(key string) bool {
	return normalizedPurposeKey(key) == PurposeTransactional
}

// PreferenceRef is a token's resolution: whose consent.
type PreferenceRef struct {
	PersonID ids.PersonID
}

// PurposeChoice is one row of the preference center: the purpose, the
// recipient's current state, and the two reasons this surface may not offer a
// change — locked at all, or grantable only through a confirmation round-trip
// it cannot perform.
type PurposeChoice struct {
	Key   string
	Label string
	State string
	// Locked: no change at all from this surface.
	Locked bool
	// GrantNeedsConfirmation: withdrawing works, granting does not. Carried to
	// the client so the switch is never OFFERED as a grant, because the write
	// refuses it and a control that always fails is worse than an absent one.
	GrantNeedsConfirmation bool
}

// preferenceTokenTTLDays is how long one preference link stays honoured
// after the message that carried it (0144). Generous where its sibling
// doiTokenTTL is short, because the two credentials answer opposite
// questions: an unclicked confirmation is a refusal, while an unsubscribe
// link a recipient reaches for weeks later must still work. The send path
// slides it forward on every message, so anyone still receiving mail always
// holds a live link.
const preferenceTokenTTLDays = 30

// preferenceTokenMaxAgeDays is the ceiling the slide cannot raise: past it
// the send path retires the token and mints a fresh one, so a leaked copy is
// bounded even for a recipient who receives mail forever. Without it the
// slide alone would leave exactly the population at risk — an active bulk-mail
// subscriber — holding one permanent credential, which is the defect 0144
// exists to end. The residue is honest and bounded: a token retired at the
// ceiling is revoked immediately, but one whose recipient stops receiving
// mail just before it simply ages out, so the worst case is this ceiling plus
// one TTL.
const preferenceTokenMaxAgeDays = 180

// ResolvePreferenceToken answers which recipient a public preference link
// speaks for. An unknown, revoked or expired token reads as absent, all three
// identically, so the surface never becomes an oracle for which of the three
// it was.
//
// The token used to answer WHICH TENANT as well — preference_token sat outside
// row-level security (0048) precisely because it was the resolver for a
// surface that has no session. There is one installation now, and the identity
// middleware binds it into every request context before this runs.
func (s *Store) ResolvePreferenceToken(ctx context.Context, token string) (PreferenceRef, error) {
	var ref PreferenceRef
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT person_id FROM preference_token
			 WHERE token = $1 AND revoked_at IS NULL AND expires_at > now()`,
			token).Scan(&ref.PersonID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	})
	if err != nil {
		return PreferenceRef{}, err
	}
	return ref, nil
}

// PreferenceTokenForEmail resolves a recipient address to their live
// preference token, minting one lazily on first use, so the send path can
// build the List-Unsubscribe URL. An address no person carries yields no
// token (found=false): the send would fail the consent gate anyway, so
// nothing is disclosed. The lookup carries its own workspace predicate —
// core 0217 retired the policy that used to supply one — and the row-scope
// probe below scopes it to the caller.
func (s *Store) PreferenceTokenForEmail(ctx context.Context, email string) (token string, found bool, err error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return "", false, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The SAME resolution the send gate applies (gate.go resolvePerson), and
		// it has to be: this mints the unsubscribe credential for a send the
		// gate has already authorized against one person, so a lookup that can
		// name a different one puts that person's link in this recipient's
		// mailbox.
		//
		// Only a LIVE address resolves. uq_person_email_dedupe is partial on
		// archived_at IS NULL, so one string can sit archived on one person and
		// live on another — and the archived arm belongs to nobody who currently
		// holds it.
		//
		// Ambiguity refuses rather than picks, for the reason the gate gives:
		// a bare LIMIT 1 over two live matches is a silent choice between two
		// people made by row order. The dedupe index should make that
		// impossible; this refuses anyway rather than trusting an invariant it
		// does not check.
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT pe.person_id
			FROM person_email pe
			JOIN person p ON p.id = pe.person_id AND p.archived_at IS NULL
			WHERE lower(pe.email) = $1 AND pe.archived_at IS NULL
			LIMIT 2`, email)
		if err != nil {
			return err
		}
		matches, err := pgx.CollectRows(rows, pgx.RowTo[ids.PersonID])
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return nil // not a known recipient in this workspace: no token, no header
		}
		if len(matches) > 1 {
			return fmt.Errorf("consent: the recipient address is live on more than one person, so no unsubscribe link can name which: %w",
				apperrors.ErrConflict)
		}
		personID := matches[0]
		// The token this mints is a bearer credential over the recipient's
		// consent record — it reads their per-purpose state, withdraws, and
		// grants, all with no session. So the mint carries the SAME row-scope
		// probe the sibling read applies (PublicPurposeStates): the object
		// grant above says the caller may read people, this says they may read
		// THIS one. Without it a row_scope=own seat obtains durable authority
		// over a person who 404s to them on every authenticated surface.
		//
		// A row-scope miss refuses the send (404, existence-hiding) rather
		// than falling through to found=false: that branch means "this address
		// carries no unsubscribe surface", and answering it here would
		// transmit marketing mail with no working List-Unsubscribe URL —
		// trading a credential leak for an RFC 8058 violation.
		//
		// The STRICT twin, because this mint creates a capability rather than
		// answering a read. Statements in a read-committed transaction each
		// take a fresh snapshot, so an Art. 17 erasure committing between the
		// lookup above and this probe would leave the plain EnsureVisible
		// answering "yes, still yours" for the tombstone — its own doc names
		// that case — and this path would then mint a NEW public credential
		// for the subject whose old one the erasure just deleted.
		if err := auth.EnsureVisibleLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		found = true
		token, err = ensurePreferenceTokenTx(ctx, tx, personID)
		return err
	})
	if err != nil {
		return "", false, err
	}
	return token, found, nil
}

// ensurePreferenceTokenTx returns the token this message's unsubscribe link
// will carry: the person's existing one when it is still honourable, a fresh
// one when it is not. The partial unique index guarantees at most one live
// token per person; a concurrent minter that wins the INSERT is read back
// rather than duplicated.
//
// "Honourable" is the SAME test the public resolver applies, plus the age
// ceiling, and the refresh is folded into it: the UPDATE returns a token only
// when it matched, so a token past either bound cannot be handed back by
// accident — it falls through to rotation. Reuse is deliberate (the
// preference centre is revisitable, and one message's link must keep working
// after the next one goes out); what 0144 ends is reuse without a bound.
func ensurePreferenceTokenTx(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (string, error) {
	var token string
	err := tx.QueryRow(ctx, `
		UPDATE preference_token
		   SET expires_at = now() + make_interval(days => $2)
		 WHERE person_id = $1 AND revoked_at IS NULL
		   AND expires_at > now()
		   AND created_at > now() - make_interval(days => $3)
		RETURNING token`, personID, preferenceTokenTTLDays, preferenceTokenMaxAgeDays).Scan(&token)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	// Nothing honourable left. Retire whatever still holds this person's slot
	// in the partial unique index before minting — the INSERT would otherwise
	// collide with an expired-but-unrevoked row, and a token the resolver has
	// stopped honouring must stop existing rather than linger as a row that
	// only looks live. This rotation is the production writer revoked_at was
	// declared for in 0048 and never had.
	if _, err := tx.Exec(ctx, `
		UPDATE preference_token SET revoked_at = now()
		 WHERE person_id = $1 AND revoked_at IS NULL`, personID); err != nil {
		return "", err
	}
	fresh, err := newPreferenceToken()
	if err != nil {
		return "", err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO preference_token (person_id, token, expires_at)
		VALUES ($1, $2, now() + make_interval(days => $3))
		ON CONFLICT (person_id) WHERE revoked_at IS NULL DO NOTHING
		RETURNING token`, personID, fresh, preferenceTokenTTLDays).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent send won the INSERT — read the winner. Scanned into
		// token and returned only after, so the caller receives the winning
		// value rather than the zero one this scan is about to overwrite.
		if err := tx.QueryRow(ctx, `
			SELECT token FROM preference_token
			 WHERE person_id = $1 AND revoked_at IS NULL`, personID).Scan(&token); err != nil {
			return "", err
		}
		return token, nil
	}
	return token, err
}

func newPreferenceToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("consent: preference token entropy: %w", err)
	}
	return "pref_" + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

// PublicPurposeStates is the preference center's read: every tracked
// purpose with the recipient's current state and its locked flag. The
// system principal the public middleware binds is unbounded, so the read
// answers for the resolved person; a caller without the token never
// reaches this method.
func (s *Store) PublicPurposeStates(ctx context.Context, personID ids.PersonID) ([]PurposeChoice, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []PurposeChoice
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		// requires_double_opt_in comes from the catalog rather than a constant:
		// an operator may define their own DOI purpose, and a page that only
		// knew about the seeded one would offer that switch and fail.
		rows, err := tx.Query(ctx, `
			SELECT cp.key, cp.label, coalesce(pc.state, 'unknown'), cp.requires_double_opt_in
			FROM consent_purpose cp
			LEFT JOIN person_consent pc ON pc.purpose_id = cp.id AND pc.person_id = $1
			WHERE cp.archived_at IS NULL
			ORDER BY cp.key`, personID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c PurposeChoice
			if err := rows.Scan(&c.Key, &c.Label, &c.State, &c.GrantNeedsConfirmation); err != nil {
				return err
			}
			c.Locked = LockedPurpose(c.Key)
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// PublicSetConsent records one per-purpose choice made from the preference
// center. A locked purpose is refused; every other change rides Record —
// same proof row, audit, and consent.changed event as any other consent
// write — with a distinct `preference_center` source. The mailbox-proving
// token holder is the data subject, so NeverOverrideExisting is NOT set:
// an explicit re-grant is their own opt-in rather than a machine's guess.
//
// The two halves part company once the subject is archived. A withdrawal
// still applies — Record admits one against any subject — while a re-grant is
// refused, because an anonymized person goes on accruing consent rows through
// a capability their erasure destroyed. UpdatePreferences records the
// withdrawals in a save before its grants for that reason, so a refused
// re-grant costs the re-grant and nothing beside it.
func (s *Store) PublicSetConsent(ctx context.Context, personID ids.PersonID, purposeKey, newState string, wording *string) (State, error) {
	purposeKey = normalizedPurposeKey(purposeKey)
	if LockedPurpose(purposeKey) {
		return State{}, &ValidationError{Field: "purpose_key", Reason: "transactional consent is locked and cannot be changed from the preference center"}
	}
	purposeID, err := s.purposeByKey(ctx, purposeKey)
	if err != nil {
		return State{}, err
	}
	source := "preference_center"
	return s.Record(ctx, RecordInput{
		PersonID:   personID,
		PurposeID:  purposeID,
		NewState:   newState,
		Source:     &source,
		PolicyText: wording,
	})
}

// purposeByKey resolves a purpose key to its id within the bound
// workspace; an unknown key is a client fault, not a 500.
func (s *Store) purposeByKey(ctx context.Context, key string) (ids.PurposeID, error) {
	var id ids.PurposeID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = purposeByKeyTx(ctx, tx, key)
		return err
	})
	return id, err
}

// purposeByKeyTx is the same resolution for a caller that already holds the
// transaction — the confirm submit, which resolves the marketing purpose in the
// same commit that spends the link.
func purposeByKeyTx(ctx context.Context, tx pgx.Tx, key string) (ids.PurposeID, error) {
	var id ids.PurposeID
	err := tx.QueryRow(ctx,
		`SELECT id FROM consent_purpose WHERE key = $1 AND archived_at IS NULL`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.PurposeID{}, &ValidationError{Field: "purpose_key", Reason: "not a tracked consent purpose"}
	}
	return id, err
}
