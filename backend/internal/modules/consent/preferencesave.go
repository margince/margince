// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The preference centre's granular save, in ONE transaction.
//
// The rule that shapes this file: a refused GRANT must never cost the
// WITHDRAWAL saved beside it. Record admits a suppression against any
// subject and refuses a claim for an archived one, so a save that simply
// aborted on the first refusal would drop the opt-out of the person who
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

	"github.com/jackc/pgx/v5"

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
// a proof row says which surface the person used.
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
	ctx context.Context, personID ids.PersonID, choices []PreferenceChoiceInput,
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
		refused = nil
		for _, pass := range []ConsentState{StateWithdrawn, StateGranted} {
			for _, c := range choices {
				if c.State != pass {
					continue
				}
				outcome, applied, err := s.saveChoiceTx(ctx, tx, personID, c)
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
	ctx context.Context, tx pgx.Tx, personID ids.PersonID, c PreferenceChoiceInput,
) (ChoiceOutcome, bool, error) {
	purposeID, err := purposeByKeyTx(ctx, tx, c.PurposeKey)
	if err != nil {
		return ChoiceOutcome{}, false, err
	}
	source := sourcePreferenceCenter
	in := RecordInput{
		PersonID:   personID,
		PurposeID:  purposeID,
		NewState:   string(c.State),
		Source:     &source,
		PolicyText: c.Wording,
	}
	sub, state, err := admitRecord(ctx, in)
	if err != nil {
		return ChoiceOutcome{}, false, err
	}
	if _, err := s.recordAdmittedTx(ctx, tx, in, sub, state); err != nil {
		// A GRANT for an archived subject is refused by the live-row probe,
		// which answers ErrNotFound. On this surface that is not "no such
		// record" — the token just resolved it — so it must not reach the
		// client as a 404, and it must not take the rest of the save with it.
		if c.State == StateGranted && errors.Is(err, apperrors.ErrNotFound) {
			return ChoiceOutcome{PurposeKey: c.PurposeKey, Reason: ReasonCannotGrant}, false, nil
		}
		return ChoiceOutcome{}, false, err
	}
	return ChoiceOutcome{}, true, nil
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
	ctx context.Context, personID ids.PersonID, purposeKeys []string,
) ([]string, error) {
	var changed []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		changed = nil
		for _, key := range purposeKeys {
			key = normalizedPurposeKey(key)
			if LockedPurpose(key) {
				return &ValidationError{
					Field:  "purpose",
					Reason: "transactional consent is locked and cannot be withdrawn",
				}
			}
			purposeID, err := purposeByKeyTx(ctx, tx, key)
			if err != nil {
				return err
			}
			source := sourcePreferenceCenter
			in := RecordInput{
				PersonID:  personID,
				PurposeID: purposeID,
				NewState:  string(StateWithdrawn),
				Source:    &source,
			}
			sub, state, err := admitRecord(ctx, in)
			if err != nil {
				return err
			}
			out, err := s.recordAdmittedTx(ctx, tx, in, sub, state)
			if err != nil {
				return err
			}
			if out.Changed {
				changed = append(changed, key)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return changed, nil
}
