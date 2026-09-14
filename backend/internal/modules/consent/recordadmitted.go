// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Writing one admitted consent decision: the row probe the state chooses, the
// idempotence check, the proof, the audit image and the event.
//
// Split from store.go because that file had reached the length ceiling, and
// this is the one concept in it big enough to stand alone — every caller in the
// module funnels a grant or a withdrawal through here, so the file is what
// "recording a decision" means.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordAdmittedTx is the write itself, on input Record has already admitted —
// so admission runs once per request and its authorization check is never
// evaluated twice.
func (s *Store) recordAdmittedTx(
	ctx context.Context, tx pgx.Tx, in RecordInput, sub subject, state ConsentState,
) (State, error) {
	actor, _ := principal.Actor(ctx)

	var out State
	probe, err := rowProbeFor(state)
	if err != nil {
		return State{}, err
	}
	if err := probe(ctx, tx, sub.entityType, sub.id); err != nil {
		return State{}, err
	}
	purposeKey, requiresDOI, err := loadConsentPurpose(ctx, tx, in.PurposeID)
	if err != nil {
		return State{}, err
	}
	// FOR UPDATE, because what follows is a read-modify-write and the decision
	// it makes is "nothing to do". Without the lock this read sees the state
	// the transaction started with, so a write to the same (subject, purpose)
	// committing in the window is invisible: the idempotence check below
	// answers no-op and the other write stands. An "unsubscribe from
	// everything" pressed while a confirmation round-trip lands is exactly
	// that shape, and it reported success while leaving the lane running.
	//
	// A missing row locks nothing and need not: the INSERT ... ON CONFLICT
	// below blocks on the conflicting insert instead, and updates.
	var current string
	err = tx.QueryRow(ctx,
		`SELECT state FROM contact_consent WHERE `+sub.column+` = $1 AND purpose_id = $2 FOR UPDATE`,
		sub.id, in.PurposeID).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return State{}, err
	}
	// AN UNCONFIRMED GRANT IS NOT THE SAME STATE, however the column reads.
	//
	// The row says `granted` and the send gate refuses it, because a
	// double-opt-in purpose is only effective once the round trip happened
	// (authorizelead.go). So a subject spending the confirmation link for a
	// grant already sitting there unconfirmed was answered as an idempotent
	// re-assertion — no proof row, no confirmation recorded — and the message
	// they were confirming still never went.
	//
	// It is the shape a preference-centre grant used to leave behind, and
	// exactly what resubscribe.go now mints a link to repair. Short-circuiting
	// here would have made that repair a no-op.
	reassertion := current == in.NewState
	if reassertion && requiresDOI && in.NewState == string(StateGranted) {
		confirmed, err := grantIsConfirmedTx(ctx, tx, sub, in.PurposeID)
		if err != nil {
			return State{}, err
		}
		reassertion = confirmed
	}
	if reassertion {
		out = State{PurposeID: in.PurposeID, PurposeKey: purposeKey, State: current, LawfulBasis: in.LawfulBasis}
		return out, nil // idempotent re-assertion: no proof row, no event, no fresh token demanded (Changed stays false)
	}
	if in.NeverOverrideExisting && current != "" {
		out = State{PurposeID: in.PurposeID, PurposeKey: purposeKey, State: current}
		return out, nil // the decision on record stands; an anonymous capture cannot flip it
	}

	doiConfirmedAt, err := s.resolveDOIConfirmation(ctx, tx, in, sub, requiresDOI)
	if err != nil {
		return State{}, err
	}

	capturedAt := s.now().UTC()
	if err := upsertConsentWithProof(ctx, tx, in, sub, doiConfirmedAt, capturedAt, actor.ID); err != nil {
		return State{}, err
	}

	action := "consent_grant"
	if ConsentState(in.NewState) == StateWithdrawn {
		action = "consent_withdraw"
	}
	auditID, err := storekit.Audit(ctx, tx, action, sub.entityType, sub.id, map[string]any{fieldState: stateOrUnknown(current)}, map[string]any{
		fieldKeyPurpose: purposeKey, fieldState: in.NewState,
	})
	if err != nil {
		return State{}, err
	}
	if err := storekit.EmitEventForEntity(ctx, tx, auditID, sub.entityType, sub.id,
		consentChangedPayload(in.PurposeID, purposeKey, in.NewState)); err != nil {
		return State{}, err
	}
	out = State{
		PurposeID: in.PurposeID, PurposeKey: purposeKey, State: in.NewState,
		LawfulBasis: in.LawfulBasis, DoubleOptInConfirmedAt: doiConfirmedAt, UpdatedAt: &capturedAt,
		Changed: true,
	}
	return out, nil
}

// rowProbeFor answers which row probe a recorded state must clear.
//
// A WITHDRAWAL stays recordable against an archived — including an Art. 17
// anonymized — subject: suppression is what you most want still working once
// somebody has asked to be forgotten.
//
// A GRANT does not. Anonymize-in-place leaves the contact row standing, so an
// erased subject would go on accruing contact_consent, consent_event, audit and
// outbox rows — the accrual erasure destroys the emailed capabilities to stop.
// This closes it from the other end.
//
// "Permissive" is weaker than it sounds: EnsureWritable runs NO statement for
// an actor unbounded on the table, and every human is unbounded on `lead` — so
// that arm is ungated for a lead, and gated for a contact only by capture
// privacy. Nothing outside tests sets LeadID, which is why that is a note not a
// finding (#2574).
//
// Exhaustive rather than defaulted: a state added to ParseRecordableState must
// come here and say whether it is a claim or a suppression, and is refused
// until it does.
func rowProbeFor(state ConsentState) (func(context.Context, pgx.Tx, string, ids.UUID) error, error) {
	switch state {
	case StateGranted:
		return auth.EnsureWritableLive, nil
	case StateWithdrawn:
		return auth.EnsureWritable, nil
	default:
		// Not a bad request: ParseRecordableState already admitted this value,
		// so arriving here means the vocabulary grew and this decision was not
		// made. That is a defect in the code, and it refuses rather than
		// guessing which probe the new state wants.
		return nil, fmt.Errorf("consent: %q is recordable but no row probe has been chosen for it — decide whether it is a lawful-basis claim (live subject only) or a suppression (any subject)", state)
	}
}

// consentChangedPayload builds the consent.changed wire payload — the
// subject travels separately (sub.entityType/sub.id, passed to
// storekit.EmitEventForEntity), since this event's entity is dynamic
// (contact XOR lead): the payload itself only ever carries the
// purpose/state triple.
func consentChangedPayload(purposeID ids.PurposeID, purposeKey, newState string) crmcontracts.PublicEventConsentChanged {
	return crmcontracts.PublicEventConsentChanged{
		PurposeId: openapi_types.UUID(purposeID.UUID),
		Purpose:   purposeKey,
		NewState:  newState,
	}
}
