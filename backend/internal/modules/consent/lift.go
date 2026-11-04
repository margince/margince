// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Lifting a suppression, and who may.
//
// judgeLift (stopcorrection.go) decides it, and this file does not ask again.
// A second comparison here would stop matching the day a level is added. Then
// it would decide whether mail reaches somebody who asked us to stop.

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
	ContactID ids.ContactID
	// SuppressionID is the row, not the contact: a subject may carry more than
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
	// What is judged and what is stored are the same words.
	in.Reason = strings.TrimSpace(in.Reason)
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
	sub, err := consentSubject(RecordInput{ContactID: in.ContactID})
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
	if err := admitLifter(ctx); err != nil {
		return subject{}, "", err
	}
	return sub, authorityOf(ctx), nil
}

// admitLifter is the caller half of a lift, which the list of stops asks too:
// a human signed in to a seat, holding contact:update. auth.RequireHuman would
// admit the system and connector principals, and authorityOf reads an admin
// role off any principal, so the type is asked here by name.
func admitLifter(ctx context.Context) error {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalHuman {
		return fmt.Errorf("only a person signed in to a seat takes back a stop: %w", apperrors.ErrPermissionDenied)
	}
	return auth.Require(ctx, "contact", principal.ActionUpdate)
}

// mayLiftOn answers the caller half of a lift for one contact without failing
// the read: false where Lift would refuse with a 403 or 404.
func mayLiftOn(ctx context.Context, tx pgx.Tx, contact ids.UUID) (bool, error) {
	err := admitLifter(ctx)
	if err == nil {
		err = auth.EnsureRetractable(ctx, tx, entityContact, contact)
	}
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
		return false, nil
	}
	return false, err
}

// fieldReason names the reason field once, so the two refusals and the audit
// payload spell it the same way.
const fieldReason = "reason"

// reasonMax bounds a reason. It is the contract's own maxLength written again
// here because nothing generated enforces it — unchecked, one caller stores a
// megabyte in an audit payload every later reader of this contact's history is
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
	// FIRST, before anything that reads the subject's row. The count below and
	// the level read want every concurrent writer of this subject's stops to be
	// committed or not-yet-started, never half-applied — and the lock has to be
	// the transaction's FIRST, because a merge holds the contact row while it
	// reaches for this same advisory lock. Taking EnsureRetractable first would
	// invert the order between the two and deadlock; see suppress.go, where it
	// did exactly that.
	if err := lockSubjectSuppressions(ctx, tx, sub.id); err != nil {
		return err
	}
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

	// BY ID, NOT BY PURPOSE — deliberately. A row's purpose_id (nil for a
	// broad stop, a consent_purpose id for a narrow one) is not asked here at
	// all, because LiftInput already names the one row to take back: whatever
	// it binds, lifting IT is what was requested. A purpose-aware clause would
	// only ever narrow which rows this matches, and the id already picks
	// exactly one — there is nothing left for a purpose filter to do except
	// occasionally answer not-found for a row that does exist.
	var decided, kind, origin, originSource string
	err = tx.QueryRow(ctx, `
		SELECT s.decided_by_level, s.kind, `+originColumnSQL("s", "captured_by")+`,
		       `+originColumnSQL("s", "source")+`
		  FROM communication_suppression s
		 WHERE s.id = $1 AND s.contact_id = $2 AND s.revoked_at IS NULL
		 FOR UPDATE`, in.SuppressionID, sub.id).Scan(&decided, &kind, &origin, &originSource)
	if errors.Is(err, pgx.ErrNoRows) {
		// A row that is already revoked, belongs to another subject, or never
		// existed all answer alike: a caller learns nothing about rows they were
		// not going to be allowed to touch.
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("consent: reading the suppression: %w", err)
	}

	verdict := judgeLift(level, stopFacts{
		kind: kind, decided: commsauthz.AuthorityLevel(decided), origin: origin, originSource: originSource,
	})
	if !verdict.may {
		return fmt.Errorf("%s: %w", liftRefusal(verdict.blockedBy), apperrors.ErrPermissionDenied)
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
	// Two lifts of DIFFERENT rows on one contact would otherwise each see the
	// other's row still live and both emit still_suppressed=true — and nothing
	// emits again afterwards, so both consumers hold mail forever on a contact
	// with no stop left. "A moment of over-counting" was the wrong reading: an
	// event is not a poll, and there is no later correction.
	//
	// So the whole contact is taken first, and every lift of the same contact
	// queues behind it. One contact's lock, held for the length of one lift —
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
	// Contact AND address, not contact alone. A row pinned to an address carries
	// no contact_id, so counting by contact would report "nothing stands" while
	// the engine still refuses every message to that mailbox. Reporting fewer
	// stops than exist is the one error this field must not make: it is the
	// direction that resumes mail.
	//
	// liveSuppression carries a third arm, lead_id, because it is called with
	// either a contact or a lead. This is not: LiftInput takes a ContactID and
	// consentSubject resolves it, so a lead arm here would be a clause that can
	// never match — the appearance of mirroring the engine without the fact.
	//
	// PURPOSE-AGNOSTIC on the same reasoning as the row lookup above: this asks
	// whether ANY live stop remains, and a narrow one bound to a single
	// newsletter is as much "still suppressed" as a broad one — the subject is
	// still stopped from something, which is exactly what still_suppressed
	// exists to tell a reader. Narrowing the count by purpose would answer
	// "nothing stands" while a narrow objection the lift never touched keeps
	// refusing that one newsletter, which is under-reporting in the one
	// direction this field must not fail: the direction that resumes mail.
	//
	// Held by TestALiftReportsAnAddressPinnedStopAsStanding.
	var remaining int
	if err = tx.QueryRow(ctx, `
		SELECT count(*) FROM communication_suppression s
		 WHERE `+contactLiveStopsSQL, sub.id).Scan(&remaining); err != nil {
		return fmt.Errorf("consent: counting the stops still standing: %w", err)
	}

	audit := map[string]any{
		"lifted_suppression": in.SuppressionID.String(),
		"recorded_at_level":  decided,
		"lifted_by_level":    string(level),
		"lifted_by":          by,
		// The lifter's words. Suppress keeps the subject's words off the
		// audit payload; this reason is the installation explaining itself.
		fieldReason: in.Reason,
	}
	if verdict.correction {
		audit[auditFieldLiftBasis] = liftBasisCorrection
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "update", sub.entityType, sub.id, audit)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, sub.id,
		suppressionLiftedPayload(in.SuppressionID, decided, level, remaining))
}

// contactLiveStopsSQL selects the live stops of the contact in $1, over the
// suppression aliased `s`. It takes the contact's own rows and the rows held on
// one of its live addresses. The lift's count and the list of stops share it.
const contactLiveStopsSQL = `s.revoked_at IS NULL
		   AND (s.contact_id = $1
		        OR (s.address IS NOT NULL AND EXISTS (
		              SELECT 1 FROM contact_email pe
		               WHERE pe.contact_id = $1
		                 AND pe.archived_at IS NULL
		                 AND pe.email = lower(s.address))))`

// auditFieldLiftBasis marks a lift that corrected a stop recorded by mistake,
// so the history can tell it from an ordinary lift.
const (
	auditFieldLiftBasis = "lift_basis"
	liftBasisCorrection = "recorded_by_mistake"
)

// liftRefusal says, in the 403's words, why this stop is not the caller's.
func liftRefusal(blockedBy crmcontracts.ContactStopLiftBlockedBy) string {
	switch blockedBy {
	case crmcontracts.ContactStopLiftBlockedByRestriction:
		return "a processing restriction is lifted only after the person is told in advance, which Margince cannot do yet"
	case crmcontracts.ContactStopLiftBlockedBySubjectDecision:
		return "the person set this stop themselves; only they can change it"
	}
	return "this stop was recorded at a level you do not outrank"
}

// suppressionLiftedPayload says a stop was taken back and by whose authority.
//
// It carries the two levels rather than the reason, for the same reason the
// recording event does: the words belong to the contacts who wrote them, and an
// event reaches readers the explanation was not given to.
//
// It also carries what the lift did NOT do. One contact can hold several stops —
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
