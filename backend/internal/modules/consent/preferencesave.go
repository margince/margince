// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The preference centre's granular save, in ONE transaction.
//
// The rule that shapes this file: a refused GRANT must never cost the
// WITHDRAWAL saved beside it. Record admits a suppression against any
// subject and refuses a claim for an archived one, so a save that simply
// aborted on the first refusal would drop the opt-out of the contact who
// most needs it — somebody who has already asked to be forgotten and is
// now asking to be left alone. An "all or nothing" save is the obvious
// shape and the wrong one.
//
// So: one commit, withdrawals recorded before grants, and a grant the
// engine refuses is COLLECTED rather than fatal. Every other error still
// rolls the whole transaction back, because a fault is not a decision.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PreferenceChoiceInput is one row of a save, normalized: the key as the
// engine will read it and a state already parsed.
type PreferenceChoiceInput struct {
	PurposeKey string
	State      ConsentState
	Wording    *string
}

// ChoiceOutcome reports one choice the save could not record. Applied
// choices are not reported: the refreshed purpose list already carries
// them, and a caller that has to diff two lists to find the failures is
// a caller that will not bother.
type ChoiceOutcome struct {
	PurposeKey string
	// Reason is a stable code the client renders, never a sentence: the
	// page is public and its copy is translated.
	Reason string
}

// ReasonCannotGrant says the subject cannot be granted this purpose.
// Today that is an archived — including an Art. 17 anonymized — record,
// whose erasure destroyed the very capability a fresh grant would re-open.
const ReasonCannotGrant = "cannot_grant"

// fieldPurposeKey names the purpose in a validation fault and on the wire,
// so the client reads one spelling.
const fieldPurposeKey = "purpose_key"

// sourcePreferenceCenter marks every consent row this surface writes, so
// a proof row says which surface the contact used.
const sourcePreferenceCenter = "preference_center"

// settleTowardWithdrawal collapses a purpose named twice in one save onto
// its withdrawal. A body carrying both answers for one purpose is a client
// bug, and on a consent surface the safe reading of it is the suppressing
// one — never request order, which decides it by accident.
func settleTowardWithdrawal(choices []PreferenceChoiceInput) []PreferenceChoiceInput {
	withdrawn := make(map[string]bool, len(choices))
	for _, c := range choices {
		if c.State == StateWithdrawn {
			withdrawn[c.PurposeKey] = true
		}
	}
	out := make([]PreferenceChoiceInput, 0, len(choices))
	seen := make(map[string]bool, len(choices))
	for _, c := range choices {
		if seen[c.PurposeKey] {
			continue
		}
		seen[c.PurposeKey] = true
		if withdrawn[c.PurposeKey] {
			c.State = StateWithdrawn
		}
		out = append(out, c)
	}
	return out
}

// PublicSaveChoices records every choice of one save in a single
// transaction and returns the ones it could not apply.
//
// Ordering inside the commit is withdrawals then grants. With one commit
// that ordering no longer protects the withdrawals — the refusal handling
// below does — but it keeps the audit trail reading in a fixed order
// rather than in whatever order a client happened to serialize its form.
func (s *Store) PublicSaveChoices(
	ctx context.Context, contactID ids.ContactID, choices []PreferenceChoiceInput,
) ([]ChoiceOutcome, error) {
	for _, c := range choices {
		if LockedPurpose(c.PurposeKey) {
			return nil, &ValidationError{
				Field:  fieldPurposeKey,
				Reason: "transactional consent is locked and cannot be changed from the preference center",
			}
		}
	}
	var refused []ChoiceOutcome
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockOneSubjectsConsent(ctx, tx, contactID); err != nil {
			return err
		}
		refused = nil
		for _, pass := range []ConsentState{StateWithdrawn, StateGranted} {
			for _, c := range choices {
				if c.State != pass {
					continue
				}
				outcome, applied, err := s.saveChoiceTx(ctx, tx, contactID, c)
				if err != nil {
					return err
				}
				if !applied {
					refused = append(refused, outcome)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refused, nil
}

// saveChoiceTx records one choice. A refusal the engine is entitled to
// make is returned as an outcome; anything else is an error that rolls
// the save back.
// The bool is "applied": a refusal the engine is entitled to make is not
// an error, and a nil outcome beside a nil error would leave the caller
// guessing which of the two it got.
func (s *Store) saveChoiceTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, c PreferenceChoiceInput,
) (ChoiceOutcome, bool, error) {
	purposeID, err := purposeByKeyTx(ctx, tx, c.PurposeKey)
	if err != nil {
		return ChoiceOutcome{}, false, err
	}
	source := sourcePreferenceCenter
	in := RecordInput{
		ContactID:  contactID,
		PurposeID:  purposeID,
		NewState:   string(c.State),
		Source:     &source,
		PolicyText: c.Wording,
	}
	sub, state, err := admitRecord(ctx, in)
	if err != nil {
		return ChoiceOutcome{}, false, err
	}
	// Whether the subject can take a GRANT at all is asked HERE rather than
	// read off the error below. ErrNotFound is answered by more than one
	// thing in the write — a purpose archived between the key lookup and the
	// proof read answers it too — and a catch-all would report that genuine
	// fault as an ordinary "you cannot opt in", telling the recipient their
	// choice was declined when in truth nothing looked at it.
	if c.State == StateGranted {
		grantable, err := subjectTakesAGrantTx(ctx, tx, sub)
		if err != nil {
			return ChoiceOutcome{}, false, err
		}
		if !grantable {
			return ChoiceOutcome{PurposeKey: c.PurposeKey, Reason: ReasonCannotGrant}, false, nil
		}
		// A DOUBLE-OPT-IN PURPOSE NEEDS THE ROUND TRIP, and writing the grant
		// here without one produced a row the send gate refuses anyway — the
		// subject pressed subscribe, the page said yes, and nothing arrived.
		// resubscribe.go mints the confirmation instead and reports the answer
		// as pending; the grant is recorded when the link is spent.
		outcome, handled, err := s.resubscribeTx(ctx, tx, contactID, purposeID, c.PurposeKey)
		if err != nil {
			return ChoiceOutcome{}, false, err
		}
		if handled {
			return outcome, false, nil
		}
	}
	if _, err := s.recordAdmittedTx(ctx, tx, in, sub, state); err != nil {
		return ChoiceOutcome{}, false, err
	}
	return ChoiceOutcome{}, true, nil
}

// subjectTakesAGrantTx asks the one question the refusal above turns on:
// is this subject still live enough to claim a lawful basis.
//
// The same probe recordAdmittedTx runs for a grant, asked in advance so
// its refusal can be told apart from every other ErrNotFound in the write.
func subjectTakesAGrantTx(ctx context.Context, tx pgx.Tx, sub subject) (bool, error) {
	err := auth.EnsureWritableLive(ctx, tx, sub.entityType, sub.id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// lockOneSubjectsConsent serializes multi-purpose consent transactions for one
// contact.
//
// Three of them write several purposes in one transaction, and each takes the
// row lock recordAdmittedTx needs in its OWN order: the withdrawal sweeps go by
// ascending purpose key, and a granular save goes withdrawals-first so a
// refused grant cannot cost the suppression saved beside it. A save of {grant a,
// withdraw b} therefore locks b before a while an unsubscribe-everything locks
// a before b, and two of those at once on one contact deadlock — Postgres aborts
// one, and what the reader sees is a preference change that failed for no
// reason they can act on.
//
// Ordering the writes instead would mean choosing between the two orders, and
// the save's order is load-bearing. So the serialization is a lock of its own,
// taken first and held to commit: inside it, the order stops mattering.
//
// Advisory rather than a row lock, because a purpose the contact holds NO row
// for is exactly the case a row lock cannot cover — and a first grant is that
// case. The key is the contact, so two DIFFERENT subjects never wait for each
// other.
func lockOneSubjectsConsent(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, contactID); err != nil {
		return fmt.Errorf("consent: taking the subject's consent lock: %w", err)
	}
	return nil
}

// PublicWithdrawAll stops the named purposes in one transaction and
// returns ONLY the ones this call actually changed.
//
// That return is the whole point. Record is idempotent, so a replayed
// press writes nothing — but the old handler still echoed the purpose
// back, and a page cannot tell a first press from a second one if the
// answer is identical. Reporting the change makes "you are already
// unsubscribed" representable instead of showing a fresh confirmation
// for a no-op.
func (s *Store) PublicWithdrawAll(
	ctx context.Context, contactID ids.ContactID, purposeKeys []string,
) ([]string, error) {
	var changed []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockOneSubjectsConsent(ctx, tx, contactID); err != nil {
			return err
		}
		var err error
		changed, err = s.withdrawPurposesTx(ctx, tx, contactID, purposeKeys)
		return err
	})
	if err != nil {
		return nil, err
	}
	return changed, nil
}

// PublicWithdrawEverything stops every purpose a recipient can be stopped on,
// choosing them inside the transaction that stops them.
//
// The unnamed press used to read the recipient's purposes first and hand the
// list to PublicWithdrawAll, which is a selection made in one transaction and
// acted on in another: a purpose granted in that window — a confirmation
// round-trip landing on the press — was not in the snapshot, survived an
// "unsubscribe from everything", and the response said it was done.
//
// The fix is not a tighter snapshot but no snapshot: the catalog decides, and
// every live purpose that is not locked is withdrawn whether or not the
// recipient held it. Record is idempotent, so a purpose they had already
// stopped costs a read and reports no change — which is what the old filter on
// their state was buying. What it no longer buys is a stale list.
//
// A grant that commits AFTER this transaction still stands, and should: it
// post-dates the press rather than being missed by it.
func (s *Store) PublicWithdrawEverything(
	ctx context.Context, contactID ids.ContactID,
) ([]string, error) {
	var changed []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockOneSubjectsConsent(ctx, tx, contactID); err != nil {
			return err
		}
		keys, err := withdrawablePurposeKeysTx(ctx, tx)
		if err != nil {
			return err
		}
		changed, err = s.withdrawPurposesTx(ctx, tx, contactID, keys)
		return err
	})
	if err != nil {
		return nil, err
	}
	return changed, nil
}

// withdrawablePurposeKeysTx reads the live purposes a recipient may be stopped
// on. The catalog rather than a constant: an operator may define their own
// purpose, and a press that only knew the seeded ones would leave it running.
func withdrawablePurposeKeysTx(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx,
		`SELECT key FROM consent_purpose WHERE archived_at IS NULL ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		if LockedPurpose(key) {
			continue
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// withdrawPurposesTx records a withdrawal for each key and answers the ones it
// actually changed.
func (s *Store) withdrawPurposesTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, purposeKeys []string,
) ([]string, error) {
	var changed []string
	for _, key := range purposeKeys {
		key = normalizedPurposeKey(key)
		if LockedPurpose(key) {
			return nil, &ValidationError{
				Field:  fieldKeyPurpose,
				Reason: "transactional consent is locked and cannot be withdrawn",
			}
		}
		purposeID, err := purposeByKeyTx(ctx, tx, key)
		if err != nil {
			return nil, err
		}
		source := sourcePreferenceCenter
		in := RecordInput{
			ContactID: contactID,
			PurposeID: purposeID,
			NewState:  string(StateWithdrawn),
			Source:    &source,
		}
		sub, state, err := admitRecord(ctx, in)
		if err != nil {
			return nil, err
		}
		out, err := s.recordAdmittedTx(ctx, tx, in, sub, state)
		if err != nil {
			return nil, err
		}
		if out.Changed {
			changed = append(changed, key)
		}
	}
	return changed, nil
}
