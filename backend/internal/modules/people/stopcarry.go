// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What happens to a subject's STOPS when another record survives them.
//
// consentcarry.go beside this carries person_consent and its proof rows, which
// people owns. It does not carry communication_suppression, which consent
// owns — so a merge moved the grants and left the stops pointing at a record
// nothing evaluates any more.
//
// The failure is quiet and it is the worst-shaped one this domain has.
// Somebody objects to marketing. Their contact is later merged into a
// duplicate — ordinary cleanup, done by a rep who has no idea a stop exists.
// The engine evaluates the survivor, finds nothing, and marketing resumes
// against a person who explicitly refused it. Nothing in the audit says a stop
// was dropped, because nothing dropped it: it is still there, attached to an
// id no send will ever ask about.
//
// A HAND-WRITTEN OBJECTION MAKES THIS SHARPER. An Art. 21 objection is
// recorded at the subject's own authority precisely so no seat can lift it,
// and a merge undid it without anyone deciding to.
//
// WHY A PORT rather than another relink in mergerelink.go. The merge already
// relinks six sibling tables under a ratified exception that doc.go names one
// by one, and that exception is narrow on purpose. Suppressions are not a
// relink anyway: the survivor may hold their own stop, the two carry different
// authorities, and both must survive — which is a decision only consent can
// make. So people asks, and consent answers, inside people's transaction.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// StopCarrier reconciles one subject's stops onto the record that survives
// them, inside the caller's transaction.
//
// Inside the transaction because a merge that committed its relinks and then
// failed to carry the stops would leave exactly the state this exists to
// prevent, with no way to tell it from a merge that never tried.
type StopCarrier interface {
	// LockStopsTx takes consent's own lock on both subjects, and MUST be
	// called before the merge locks any person row.
	//
	// THE ORDER IS THE WHOLE REASON THIS METHOD EXISTS. Recording a stop takes
	// consent's advisory lock on the subject and then reads the person row;
	// the merge locks the person rows and then, inside CarryStopsTx, reaches
	// for that same advisory lock. Two transactions taking the same pair of
	// locks in opposite orders is a deadlock, and Postgres duly reported one.
	//
	// Splitting the lock out lets the merge take it first, so both paths
	// acquire consent's lock before any person row and the inversion cannot
	// form. Calling it is cheap when there is nothing to carry.
	LockStopsTx(ctx context.Context, tx pgx.Tx, subjects ...commsauthz.StopSubject) error

	// CarryStopsTx copies every live stop held by the retiring subject onto
	// the survivor. Idempotent: a stop the survivor already holds is left
	// alone rather than duplicated.
	CarryStopsTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error
}

// WithStopCarrier wires the consent-side seam. Compose binds it to the consent
// store.
func (s *Store) WithStopCarrier(carrier StopCarrier) *Store {
	s.stopCarrier = carrier
	return s
}

// StopCarrierNotWiredError maps to 422 rather than 500: nothing is broken, the
// installation simply cannot merge this subject safely until the seam is bound.
type StopCarrierNotWiredError struct{}

func (e *StopCarrierNotWiredError) Error() string {
	return "this record carries a recorded stop and the consent seam that would move it onto the " +
		"survivor is not wired on this installation; merging would silently resume mail somebody " +
		"asked us to stop"
}

// FieldFault carries the refusal to every surface rather than to the HTTP one
// alone. The MCP tool surface reaches this store through the datasource seam
// and never runs the REST error mapper, so a branch there would have told an
// agent merging records that the server had failed and to try again — which it
// would, forever, because the seam is still not wired.
//
// The field is the source record, because that is the one holding the stop and
// the one an operator will look at. Naming the target would send them to the
// record that has nothing wrong with it.
func (e *StopCarrierNotWiredError) FieldFault() (field, code, message string) {
	return "source_id", "stop_carrier_not_wired", e.Error()
}

// carryStopsTx is the one call site, so the refusal below cannot be forgotten
// by a second caller written later.
//
// AN UNWIRED SEAM REFUSES ONLY WHEN THERE IS SOMETHING TO LOSE.
//
// The first spelling refused every merge outright, which is the safe direction
// and too blunt: it took out 21 existing tests, and behind them every caller
// that builds a people store for a purpose with nothing to do with consent —
// a dedupe disposition, a demote, a promote preview. Refusing those protects
// nothing, because a subject with no stop has none to drop.
//
// Refusing NOTHING would restore the defect. So the question is asked of the
// data instead of the wiring: does this subject actually hold a live stop. No
// stop, nothing to carry, and the merge proceeds exactly as it did before this
// seam existed. A stop and no seam is the one case that must not proceed, and
// it is the case an operator can act on — the message names the record.
func (s *Store) carryStopsTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error {
	return carryStopsOrRefuse(ctx, tx, s.stopCarrier, from, to)
}

// lockStopsOrSkip takes consent's lock before the merge locks any person row.
// An unwired carrier has no lock to take, and the refusal for that case is
// carryStopsOrRefuse's to make once the merge knows what it would lose.
func lockStopsOrSkip(ctx context.Context, tx pgx.Tx, carrier StopCarrier, subjects ...commsauthz.StopSubject) error {
	if carrier == nil {
		return nil
	}
	return carrier.LockStopsTx(ctx, tx, subjects...)
}

// carryStopsOrRefuse is the rule itself, taking the carrier as an argument so
// the lead merge — a free function with no store in hand — cannot grow a
// second, laxer spelling of it.
func carryStopsOrRefuse(ctx context.Context, tx pgx.Tx, carrier StopCarrier, from, to commsauthz.StopSubject) error {
	if carrier != nil {
		return carrier.CarryStopsTx(ctx, tx, from, to)
	}
	held, err := holdsALiveStop(ctx, tx, from)
	if err != nil {
		return err
	}
	if held {
		return &StopCarrierNotWiredError{}
	}
	return nil
}

// holdsALiveStop asks the one question that decides whether an unwired merge
// is safe.
//
// READ-ONLY, and people does not own this table — which is why it is a bare
// EXISTS rather than anything that interprets a row. Whether a stop BINDS a
// given message is consent's judgement and lives there; whether one exists at
// all is a fact this module may look at to decide whether it is about to
// destroy something.
func holdsALiveStop(ctx context.Context, tx pgx.Tx, subject commsauthz.StopSubject) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM communication_suppression
			 WHERE revoked_at IS NULL
			   AND (($1::uuid IS NOT NULL AND person_id = $1)
			     OR ($2::uuid IS NOT NULL AND lead_id = $2)))`,
		zeroAsNull(subject.PersonID.UUID), zeroAsNull(subject.LeadID.UUID)).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("people: checking whether the retiring record holds a stop: %w", err)
	}
	return held, nil
}

// zeroAsNull sends a zero uuid as SQL NULL, so the arms above can ask "is this
// side a person or a lead" with IS NOT NULL rather than comparing against a
// sentinel that is also a legal-looking uuid.
func zeroAsNull(id ids.UUID) *ids.UUID {
	if id.IsZero() {
		return nil
	}
	return &id
}
