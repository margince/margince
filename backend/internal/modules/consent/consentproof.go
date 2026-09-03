// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// How a recorded decision becomes a proof row: which purpose was named, what
// confirmed a grant that needed confirming, and the paired write that leaves
// the current state and its append-only evidence saying the same thing.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// loadConsentPurpose resolves the target purpose's key and DOI flag; an
// unknown or archived purpose is 404.
func loadConsentPurpose(ctx context.Context, tx pgx.Tx, purposeID ids.PurposeID) (key string, requiresDOI bool, err error) {
	err = tx.QueryRow(ctx,
		`SELECT key, requires_double_opt_in FROM consent_purpose WHERE id = $1 AND archived_at IS NULL`,
		purposeID).Scan(&key, &requiresDOI)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("purpose %s: %w", purposeID, apperrors.ErrNotFound)
	}
	if err != nil {
		return "", false, err
	}
	return key, requiresDOI, nil
}

// resolveDOIConfirmation enforces the German email norm: a DOI purpose's grant
// is only effective once the double-opt-in round-trip confirmed, and the only
// thing that confirms one is the subject spending a single-use link that was
// mailed to their own live primary address. Non-DOI paths return nil.
//
// The round trip is person-keyed, so a DOI grant on a lead subject is refused
// rather than recorded unconfirmed — the lead promotes first, then confirms.
func (s *Store) resolveDOIConfirmation(ctx context.Context, tx pgx.Tx, in RecordInput, sub subject, requiresDOI bool) (*time.Time, error) {
	if ConsentState(in.NewState) != StateGranted || !requiresDOI {
		return nil, nil
	}
	if sub.entityType != "person" {
		return nil, &ValidationError{
			// The subject, not the purpose: the purpose is fine and the caller
			// cannot fix it. What they must change is which subject they named,
			// which is the field consentSubject's own refusals already use.
			Field:  "subject",
			Reason: "a double opt-in purpose needs a person subject; promote the lead before granting it",
		}
	}
	// A mailbox already proven by the single-use link that carried the subject
	// here answers what the round trip would ask, so the grant confirms on the
	// spot. The confirmation time is this moment: the click IS the confirmation.
	if in.MailboxProof.proves() {
		confirmed := s.now().UTC()
		return &confirmed, nil
	}
	// Nothing else confirms. An operator-held token used to satisfy this, which
	// meant the round trip could be completed by the same person who started it
	// — the subject's mailbox never took part, and the grant recorded a
	// confirmation that had not happened. A grant reaching here has no mailbox
	// behind it, so it is refused rather than recorded half-true.
	return nil, &ValidationError{
		Field:  "purpose_id",
		Reason: "this purpose is confirmed by the subject through a link mailed to them, not by a value a caller supplies",
	}
}

// upsertConsentWithProof writes the state row and appends the immutable
// proof row — one concept: the current state is always backed by an
// append-only consent_event that says when, how, and by whom. The upsert
// targets the subject arm's own unique key (person×purpose or
// lead×purpose); the other arm's column stays NULL.
func upsertConsentWithProof(ctx context.Context, tx pgx.Tx, in RecordInput, sub subject, doiConfirmedAt *time.Time, capturedAt time.Time, actorID string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO person_consent (`+sub.column+`, purpose_id, state, lawful_basis, captured_at, source)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (`+sub.column+`, purpose_id)
		DO UPDATE SET state = EXCLUDED.state, lawful_basis = EXCLUDED.lawful_basis,
		              captured_at = EXCLUDED.captured_at, source = EXCLUDED.source`,
		sub.id, in.PurposeID, in.NewState, in.LawfulBasis, capturedAt, in.Source); err != nil {
		return err
	}
	// issuance_trigger names the mailbox proof this grant rests on, so the chain
	// is readable from the proof row alone. Set only where a mailbox proof
	// actually stood in for the round trip: an ordinary purpose needed no
	// confirmation, so claiming the link substituted for one would overstate
	// what happened.
	//
	// It is also what the outbound gates read to tell a real confirmation from
	// a claimed one (verdict.go, gate.go). Rows the retired operator-token
	// endpoint produced carry NULL here and authorize nothing.
	var trigger *string
	if doiConfirmedAt != nil && in.MailboxProof.proves() {
		named := string(in.MailboxProof)
		trigger = &named
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO consent_event (`+sub.column+`, purpose_id, new_state, lawful_basis, source,
		                           policy_text, policy_version, double_opt_in_confirmed_at, captured_at, captured_by,
		                           issuance_trigger)
		VALUES ($1, $2, $3, $4, coalesce($5, 'api'), coalesce($6, 'recorded via API'), coalesce($7, 'v1'), $8, $9, $10, $11)`,
		sub.id, in.PurposeID, in.NewState, in.LawfulBasis, in.Source,
		in.PolicyText, in.PolicyVersion, doiConfirmedAt, capturedAt, actorID, trigger)
	return err
}
