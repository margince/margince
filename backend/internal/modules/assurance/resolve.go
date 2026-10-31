// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The answers a contact can give a finding, plus the one the system gives
// itself.
const (
	OutcomeFixedRecord   = "fixed_record"
	OutcomeAddedEvidence = "added_evidence"
	OutcomeValueCorrect  = "value_correct"
	OutcomeNotRelevant   = "not_relevant"
	OutcomeRemindLater   = "remind_later"
	OutcomeReassign      = "reassign"
	// OutcomeConditionCleared is the SYSTEM's, never a contact's: a finding
	// whose condition stopped being true resolves itself.
	OutcomeConditionCleared = "condition_cleared"
)

const subjectKindDeal = "deal"

// codeInvalid is the ParseError code for a value of the wrong shape, as
// distinct from one that is missing or out of range.
const codeInvalid = "invalid"

// codeOutOfRange refuses a moment past the ceiling, for an expiry and a
// deferral alike.
const codeOutOfRange = "out_of_range"

// slotOutcome names the answer kind, in the one place it is spelled.
const slotOutcome = "outcome"

// noExpiry is what a non-suppressing answer carries: it hides nothing, so
// there is nothing for an expiry to bound. A sentinel rather than a nil pair,
// because "no expiry" and "the check failed" are different answers and
// returning nothing twice cannot tell them apart.
var noExpiry *time.Time

// MaxSuppression is how long an answer may hide a finding.
//
// Ninety days, and capped server-side rather than trusted from the request. A
// suppressing answer hides a finding from the screens a revenue commitment is
// made from, and "the value is correct" is a claim about a value on a day —
// values change, and an uncapped suppression outlives the fact it rested on.
const MaxSuppression = 90 * 24 * time.Hour

// LiveRemindAtSQL is when a deferral actually comes back: the moment it named,
// or MaxSuppression after it was given, whichever is sooner.
//
// Read rather than trusted, because answers written before the ceiling was
// refused still name a moment past it. days is SQL the caller renders, so a
// caller binds it however its own statement binds parameters.
func LiveRemindAtSQL(answer, days string) string {
	return "LEAST(" + answer + ".remind_at, " + answer +
		".created_at + make_interval(days => " + days + "))"
}

// MaxSuppressionDays is MaxSuppression as the whole number of days a SQL
// interval takes.
func MaxSuppressionDays() int { return int(MaxSuppression / (24 * time.Hour)) }

// Resolution is somebody's answer to a finding.
type Resolution struct {
	DealID      *ids.UUID
	Outcome     string
	Reason      string
	EvidenceRef string
	RemindAt    *time.Time
	ExpiresAt   *time.Time
}

// suppressingOutcomes hide a finding from every surface, which is why they are
// the two that must name an expiry.
func suppresses(outcome string) bool {
	return outcome == OutcomeValueCorrect || outcome == OutcomeNotRelevant
}

// Resolve records an answer to a finding and closes it.
//
// The actor comes from the authenticated principal, never from the request: an
// answer is attributable to whoever gave it, and a body-supplied actor is a
// signature anybody can forge.
func (s *Store) Resolve(ctx context.Context, exceptionID ids.UUID, in Resolution, now time.Time) error {
	actor, settled, err := admitResolution(ctx, in, now)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		return s.resolveIn(ctx, tx, exceptionID, in, actor.UserID, settled, now)
	})
}

// ResolveTx is Resolve inside the caller's transaction, for an answer that also
// writes the deal: both commit together or neither does.
func (s *Store) ResolveTx(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, in Resolution, now time.Time) error {
	actor, settled, err := admitResolution(ctx, in, now)
	if err != nil {
		return err
	}
	return s.resolveIn(ctx, tx, exceptionID, in, actor.UserID, settled, now)
}

// admitResolution checks who answers and what, before any transaction, and
// returns the expiry the answer is stored with.
func admitResolution(ctx context.Context, in Resolution, now time.Time) (principal.Principal, *time.Time, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return principal.Principal{}, nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		// An answer is given BY somebody. A system principal has nobody to
		// attribute one to, and the auto-resolve path is a different door.
		return principal.Principal{}, nil, apperrors.ErrPermissionDenied
	}
	settled, err := checkResolution(in, now)
	return actor, settled, err
}

func (s *Store) resolveIn(
	ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, in Resolution, actorID ids.UUID, settled *time.Time,
	now time.Time,
) error {
	if err := ensureAnswerable(ctx, tx, exceptionID); err != nil {
		return err
	}
	if err := ensureOutcomeFitsReview(ctx, tx, exceptionID, in.Outcome); err != nil {
		return err
	}
	if err := s.applySignalDecision(ctx, tx, exceptionID, in); err != nil {
		return err
	}
	// The status the answer leaves the finding in. A deferral does not resolve
	// it. "Not now" is about when, not whether, and a deferred finding that
	// read as resolved would never come back.
	status := "resolved"
	if in.Outcome == OutcomeRemindLater {
		status = "open"
	}
	tag, err := tx.Exec(ctx, `
		UPDATE assurance_exception
		SET status = $2, updated_at = now()
		WHERE id = $1 AND status = 'open'`,
		exceptionID, status)
	if err != nil {
		return fmt.Errorf("assurance: answering the finding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Not there, or already answered: a 404 either way. Saying a finding
		// is already resolved says it exists, about a deal the caller may not
		// be able to open.
		return apperrors.ErrNotFound
	}
	// After the status moved, so the check reads the deal as it stands
	// without this review open.
	if err := s.applyReviewCheck(ctx, tx, exceptionID, in.Outcome, now); err != nil {
		return err
	}

	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	var resolutionID ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO assurance_resolution
		    (exception_id, outcome, reason, evidence_ref, remind_at,
		     expires_at, actor_id, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		exceptionID, in.Outcome, nullIfEmpty(in.Reason),
		nullIfEmpty(in.EvidenceRef), in.RemindAt, settled,
		actorID, capturedBy).Scan(&resolutionID); err != nil {
		return fmt.Errorf("assurance: recording the answer: %w", err)
	}

	auditID, err := storekit.AuditEvent(ctx, tx, "create", "assurance_resolution", resolutionID,
		map[string]any{
			"exception_id": exceptionID.String(),
			slotOutcome:    in.Outcome,
		})
	if err != nil {
		return err
	}
	// A surface counting open findings has to know one was answered, and with
	// which kind of answer. The two suppressing outcomes hide the finding. A
	// consumer treating all six alike would keep showing it.
	//
	// The reason does not ride along: it is prose the author may edit.
	event := crmcontracts.PublicEventForecastExceptionResolved{
		ExceptionId: openapi_types.UUID(exceptionID),
		Outcome:     crmcontracts.PublicEventForecastExceptionResolvedOutcome(in.Outcome),
		ActorUserId: openapi_types.UUID(actorID),
		ExpiresAt:   settled,
	}
	return storekit.EmitEvent(ctx, tx, auditID, actorID, event)
}

// A suppressing answer requires write authority over its subject.
func ensureAnswerable(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID) error {
	kinds := make([]string, 0, 2)
	dealErr := auth.Require(ctx, subjectKindDeal, principal.ActionUpdate)
	if dealErr == nil {
		kinds = append(kinds, subjectKindDeal)
	}
	if signalErr := auth.Require(ctx, subjectKindSignal, principal.ActionUpdate); signalErr == nil {
		kinds = append(kinds, subjectKindSignal)
	}
	if callErr := auth.Require(ctx, "forecast", principal.ActionCreate); callErr == nil {
		kinds = append(kinds, SubjectKindForecastCall)
	}
	if len(kinds) == 0 {
		return dealErr
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT subject_kind, subject_id FROM assurance_exception
 WHERE id = $%d AND status = 'open' AND subject_kind = ANY($%d) FOR UPDATE`, arg(exceptionID), arg(kinds))
	var kind string
	var subjectID ids.UUID
	err := tx.QueryRow(ctx, query, args...).Scan(&kind, &subjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("assurance: reading the finding: %w", err)
	}
	if kind == subjectKindSignal {
		return ensureSignalAnswerable(ctx, tx, subjectID)
	}
	if kind == SubjectKindForecastCall {
		return ensureCallAnswerable(ctx, tx, subjectID)
	}
	if kind != subjectKindDeal {
		return apperrors.ErrNotFound
	}
	if err := auth.Require(ctx, subjectKindDeal, principal.ActionUpdate); err != nil {
		return err
	}
	// Not the live probe: an answer puts nothing on the deal, and CloseDeparted
	// closes the findings of a deal that was archived.
	return auth.EnsureWritable(ctx, tx, subjectKindDeal, subjectID)
}

// ensureCallAnswerable admits the call's own author: an answer about a call
// is the author saying the number still stands, or that they called again.
func ensureCallAnswerable(ctx context.Context, tx pgx.Tx, callID ids.UUID) error {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	var authored bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM forecast_call WHERE id = $1 AND author_id = $2)`,
		callID, actor.UserID).Scan(&authored); err != nil {
		return fmt.Errorf("assurance: reading who made the call: %w", err)
	}
	if !authored {
		return apperrors.ErrNotFound
	}
	return nil
}

// ReviewOutcomes are the four answers a viability review takes. Revived is
// confirmed by the review check against the deal's evidence. Closed lost is
// recorded as not relevant once the deal is lost. An override needs a reason
// and an end date. A wait keeps the review open.
func ReviewOutcomes() []string {
	return []string{OutcomeFixedRecord, OutcomeNotRelevant, OutcomeValueCorrect, OutcomeRemindLater}
}

func ensureOutcomeFitsReview(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, outcome string) error {
	var kind string
	if err := tx.QueryRow(ctx, `SELECT type FROM assurance_exception WHERE id = $1`, exceptionID).Scan(&kind); err != nil {
		return fmt.Errorf("assurance: reading the finding's type: %w", err)
	}
	if !slices.Contains(ReviewTypes(), kind) || slices.Contains(ReviewOutcomes(), outcome) {
		return nil
	}
	return &values.ParseError{
		Field: slotOutcome, Code: "not_allowed",
		Message: "a customer review is answered by reviving the deal, closing it as lost, an override with a reason and an end date, or a wait",
	}
}

// checkResolution refuses an answer the surface cannot honour, and returns the
// expiry it will actually be stored with.
//
// The cap is applied HERE rather than trusted from the caller. A client asking
// to suppress a finding for two years is not malicious — it is a date picker
// with no maximum — and the server is where the ceiling has to live because it
// is the only place that sees every client.
func checkResolution(in Resolution, now time.Time) (*time.Time, error) {
	switch in.Outcome {
	case OutcomeFixedRecord, OutcomeAddedEvidence, OutcomeValueCorrect,
		OutcomeNotRelevant, OutcomeRemindLater, OutcomeReassign:
	case OutcomeConditionCleared:
		// The system's own. A contact claiming it would be saying the condition
		// stopped being true without anything having checked.
		return nil, &values.ParseError{
			Field: slotOutcome, Code: "not_allowed",
			Message: "condition_cleared is recorded by the check itself, not by a contact",
		}
	default:
		return nil, &values.ParseError{
			Field: slotOutcome, Code: codeInvalid,
			Message: "that is not one of the answers a finding takes",
		}
	}

	if in.Outcome == OutcomeRemindLater {
		if in.RemindAt == nil || !in.RemindAt.After(now) {
			return nil, &values.ParseError{
				Field: "remind_at", Code: "required",
				Message: "a deferral names when it comes back, or it is a dismissal",
			}
		}
		// Refused rather than silently shortened, as an expiry past the ceiling
		// is. A deferral hides the finding as an expiry does, so it cannot
		// outlive the fact it rested on either.
		if in.RemindAt.After(now.Add(MaxSuppression)) {
			return nil, &values.ParseError{
				Field: "remind_at", Code: codeOutOfRange,
				Message: "a finding may be deferred for at most 90 days: a value that was correct in May is a claim about May",
			}
		}
		return noExpiry, nil
	}

	if !suppresses(in.Outcome) {
		return noExpiry, nil
	}
	// A suppressing answer must say WHY. It hides the finding from the screens
	// a revenue commitment is made from, and the next contact to see the number
	// is owed the reason it is not flagged.
	if in.Reason == "" {
		return nil, &values.ParseError{
			Field: "reason", Code: "required",
			Message: "an answer that hides a finding says why",
		}
	}
	ceiling := now.Add(MaxSuppression)
	if in.ExpiresAt == nil {
		return &ceiling, nil
	}
	if !in.ExpiresAt.After(now) {
		return nil, &values.ParseError{
			Field: "expires_at", Code: codeOutOfRange,
			Message: "an expiry in the past suppresses nothing",
		}
	}
	// Longer than the ceiling is refused rather than silently shortened. A
	// caller who asked for a year and got ninety days without being told would
	// believe the finding stays hidden through the next two quarters.
	if in.ExpiresAt.After(ceiling) {
		return nil, &values.ParseError{
			Field: "expires_at", Code: codeOutOfRange,
			Message: "a finding may be hidden for at most 90 days: a value that was correct in May is a claim about May",
		}
	}
	return in.ExpiresAt, nil
}
