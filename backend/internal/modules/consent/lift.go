// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Lifting a suppression, and who may.
//
// Writing one was the previous change; until this one nothing could take it
// back, so every stop a rep recorded was permanent whatever level it carried.
// That made the level a claim rather than a rule — the contract said an admin
// could lift a rep's row and no code could.
//
// ONE RULE, and it is commsauthz.AuthorityLevel.CanOverrule: you may lift a
// decision made BELOW your level, never at or above it. This file asks that
// question and does not re-answer it. A second comparison here — a rank check,
// a role switch, an "is admin" shortcut — would be the copy that stops matching
// the day a level is added, and the one that stops matching decides whether
// mail reaches somebody who asked us to stop.
//
// The subject's own act is the tier nothing satisfies. An Art. 21 objection and
// a withdrawal are theirs, not the installation's, so no seat lifts them and
// this door does not offer a way to try.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// LiftInput names the suppression to take back and why.
type LiftInput struct {
	PersonID ids.PersonID
	// SuppressionID is the row, not the person: a subject may carry more than
	// one stop, and lifting "the suppression" would silently take back whichever
	// the query happened to return first.
	SuppressionID ids.UUID
	// Reason is why it is being lifted, in the lifter's words. A stop that gets
	// taken back is the change most worth explaining — the row said somebody
	// asked us not to write, and this says why we are writing again.
	Reason string
}

// Lift revokes one suppression, if this caller outranks the level that wrote it.
func (s *Store) Lift(ctx context.Context, in LiftInput) error {
	sub, level, err := admitLift(ctx, in)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return s.liftAdmittedTx(ctx, tx, in, sub, level)
	})
}

// admitLift settles what is decidable before a connection is taken.
func admitLift(ctx context.Context, in LiftInput) (subject, commsauthz.AuthorityLevel, error) {
	sub, err := consentSubject(RecordInput{PersonID: in.PersonID})
	if err != nil {
		return subject{}, "", err
	}
	if in.SuppressionID.IsZero() {
		return subject{}, "", &ValidationError{
			Field:  "suppression_id",
			Reason: "name the suppression to lift; a subject may carry more than one",
		}
	}
	if err := requireReason(in.Reason, "taking back a stop"); err != nil {
		return subject{}, "", err
	}
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return subject{}, "", err
	}
	return sub, authorityOf(ctx), nil
}

// fieldReason names the reason field once, so the two refusals and the audit
// payload spell it the same way.
const fieldReason = "reason"

// reasonMax bounds a reason. It is the contract's own maxLength written again
// here because nothing generated enforces it — unchecked, one caller stores a
// megabyte in an audit payload every later reader of this person's history is
// served in full.
const reasonMax = 500

// boundReason holds the LENGTH rule, which both doors that take a reason share.
// Only the bound: whether a reason must be given at all differs between them,
// and the contract is what says so.
func boundReason(reason string) error {
	if n := len([]rune(reason)); n > reasonMax {
		return &ValidationError{
			Field:  fieldReason,
			Reason: fmt.Sprintf("a reason is at most %d characters; this one is %d", reasonMax, n),
		}
	}
	return nil
}

// requireReason is the LIFT's rule: a reason is mandatory here and optional on
// the door that records a stop. That asymmetry is the contract's, not an
// oversight — `crm.yaml` marks the suppress body `required: [kind]` and the
// lift body's own description says "Required, unlike the reason for setting
// one". A rep relaying "please stop emailing me" can record that stop with no
// explanation; taking one back has to be explainable.
//
// Sharing the bound and not the requirement is the whole point of the split:
// unifying both would have turned a conforming suppress request into a 422.
func requireReason(reason, act string) error {
	if strings.TrimSpace(reason) == "" {
		return &ValidationError{
			Field:  fieldReason,
			Reason: act + " needs a reason somebody can review later",
		}
	}
	return boundReason(reason)
}

// liftAdmittedTx reads the row's authority, compares it, and revokes.
func (s *Store) liftAdmittedTx(
	ctx context.Context, tx pgx.Tx, in LiftInput, sub subject, level commsauthz.AuthorityLevel,
) error {
	// EnsureRetractable, which IS EnsureWritable and says so: this is a write
	// that RELEASES rather than adds, and it reaches an archived subject on
	// purpose. The named spelling is what distinguishes that deliberate reach
	// from a live filter somebody forgot.
	if err := auth.EnsureRetractable(ctx, tx, sub.entityType, sub.id); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}

	// The row is read INSIDE the transaction that revokes it, so the level this
	// decision rests on is the level still on the row when the write lands. Read
	// outside and a concurrent lift could change it between the check and the
	// update — the shape that lets a rep's request act on an admin's answer.
	// Before the read, so the count below observes every concurrent lift of this
	// person as committed or not-yet-started, never as half-applied.
	if _, err = tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, sub.id.String()); err != nil {
		return fmt.Errorf("consent: serialising lifts for this person: %w", err)
	}

	var decided string
	err = tx.QueryRow(ctx, `
		SELECT decided_by_level FROM communication_suppression
		 WHERE id = $1 AND person_id = $2 AND revoked_at IS NULL
		 FOR UPDATE`, in.SuppressionID, sub.id).Scan(&decided)
	if errors.Is(err, pgx.ErrNoRows) {
		// A row that is already revoked, belongs to another subject, or never
		// existed all answer alike: a caller learns nothing about rows they were
		// not going to be allowed to touch.
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("consent: reading the suppression: %w", err)
	}

	if !level.CanOverrule(commsauthz.AuthorityLevel(decided)) {
		return fmt.Errorf(
			"this stop was recorded at a level you do not outrank: %w", apperrors.ErrPermissionDenied)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE communication_suppression
		   SET revoked_at = now()
		 WHERE id = $1 AND revoked_at IS NULL`, in.SuppressionID); err != nil {
		return fmt.Errorf("consent: revoking the suppression: %w", err)
	}

	// Counted AFTER the revoke and inside the same transaction, so the number
	// describes this lift's own snapshot rather than the world before it. Not a
	// serialized by the FOR UPDATE above, which locks only the row being lifted.
	// Two lifts of DIFFERENT rows on one person would otherwise each see the
	// other's row still live and both emit still_suppressed=true — and nothing
	// emits again afterwards, so both consumers hold mail forever on a person
	// with no stop left. "A moment of over-counting" was the wrong reading: an
	// event is not a poll, and there is no later correction.
	//
	// So the whole person is taken first, and every lift of the same person
	// queues behind it. One person's lock, held for the length of one lift —
	// this is not a table lock and it does not touch anybody else's subject.
	//
	// NOT held by a test, deliberately. Each Lift opens its own short
	// transaction, so two goroutines started together almost never overlap in
	// the count window: a timing test passed identically with the lock removed,
	// three runs out of three, which makes it a green that proves nothing
	// rather than a guard. Reproducing it needs two transactions held open by
	// hand around the read — worth writing when this file next gains a seam
	// that can do it, and worth nobody's confidence until then.
	//
	// Person AND address, not person alone. A row pinned to an address carries
	// no person_id, so counting by person would report "nothing stands" while
	// the engine still refuses every message to that mailbox. Reporting fewer
	// stops than exist is the one error this field must not make: it is the
	// direction that resumes mail.
	//
	// liveSuppression carries a third arm, lead_id, because it is called with
	// either a person or a lead. This is not: LiftInput takes a PersonID and
	// consentSubject resolves it, so a lead arm here would be a clause that can
	// never match — the appearance of mirroring the engine without the fact.
	//
	// Held by TestALiftReportsAnAddressPinnedStopAsStanding.
	var remaining int
	if err = tx.QueryRow(ctx, `
		SELECT count(*) FROM communication_suppression s
		 WHERE s.revoked_at IS NULL
		   AND (s.person_id = $1
		        OR (s.address IS NOT NULL AND EXISTS (
		              SELECT 1 FROM person_email pe
		               WHERE pe.person_id = $1
		                 AND pe.archived_at IS NULL
		                 AND pe.email = lower(s.address))))`,
		sub.id).Scan(&remaining); err != nil {
		return fmt.Errorf("consent: counting the stops still standing: %w", err)
	}

	auditID, err := storekit.AuditEvent(ctx, tx, "update", sub.entityType, sub.id,
		map[string]any{
			"lifted_suppression": in.SuppressionID.String(),
			"recorded_at_level":  decided,
			"lifted_by_level":    string(level),
			"lifted_by":          by,
			// The LIFTER's words, and the audit entry is their home. Suppress
			// keeps the SUBJECT's reason off the audit payload because those
			// words are theirs; this reason is the installation explaining
			// itself, and the trail is exactly who should be able to read it.
			fieldReason: in.Reason,
		})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, sub.id,
		suppressionLiftedPayload(in.SuppressionID, decided, level, remaining))
}

// suppressionLiftedPayload says a stop was taken back and by whose authority.
//
// It carries the two levels rather than the reason, for the same reason the
// recording event does: the words belong to the people who wrote them, and an
// event reaches readers the explanation was not given to.
//
// It also carries what the lift did NOT do. One person can hold several stops —
// their own objection and a rep's separate note — and lifting one leaves the
// others standing. An event saying only "a stop was lifted" reads as "you may
// write to them now", so `still_suppressed` states the opposite plainly and
// `remaining` says how many reasons remain. The engine never needed this (it
// re-reads the strongest live row when it sends); the readers outside do.
func suppressionLiftedPayload(
	lifted ids.UUID,
	recorded string,
	by commsauthz.AuthorityLevel,
	remaining int,
) crmcontracts.PublicEventConsentSuppressionLifted {
	// Always populated. The contract marks them optional so a consumer written
	// against the older shape keeps validating, not because this installation
	// may omit them: a payload that left them out would be one a reader could
	// only interpret as "unknown", and there is no case here where the answer
	// is unknown.
	id := openapi_types.UUID(lifted)
	stands := remaining > 0
	return crmcontracts.PublicEventConsentSuppressionLifted{
		SuppressionId:         &id,
		RecordedAtLevel:       recorded,
		LiftedByLevel:         string(by),
		RemainingSuppressions: &remaining,
		StillSuppressed:       &stands,
	}
}
