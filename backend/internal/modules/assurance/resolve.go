// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"
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

// The answers a person can give a finding, plus the one the system gives
// itself.
const (
	OutcomeFixedRecord   = "fixed_record"
	OutcomeAddedEvidence = "added_evidence"
	OutcomeValueCorrect  = "value_correct"
	OutcomeNotRelevant   = "not_relevant"
	OutcomeRemindLater   = "remind_later"
	OutcomeReassign      = "reassign"
	// OutcomeConditionCleared is the SYSTEM's, never a person's: a finding
	// whose condition stopped being true resolves itself.
	OutcomeConditionCleared = "condition_cleared"
)

// codeInvalid is the ParseError code for a value of the wrong shape, as
// distinct from one that is missing or out of range.
const codeInvalid = "invalid"

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

// Resolution is somebody's answer to a finding.
type Resolution struct {
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
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		// An answer is given BY somebody. A system principal has nobody to
		// attribute one to, and the auto-resolve path is a different door.
		return apperrors.ErrPermissionDenied
	}
	settled, err := checkResolution(in, now)
	if err != nil {
		return err
	}

	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		// The status the answer leaves the finding in. A deferral does NOT
		// resolve it: "not now" is an answer about when, not about whether, and
		// a deferred finding that read as resolved would never come back.
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
			// Not there, or already answered. A 404 either way: telling a
			// caller that a finding exists but is already resolved says the
			// finding exists, about a deal they may not be able to open.
			return apperrors.ErrNotFound
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
			actor.UserID, capturedBy).Scan(&resolutionID); err != nil {
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
		// A surface counting open findings has to know one was answered, and
		// WHICH KIND of answer it was: the two suppressing outcomes hide the
		// finding, and a consumer treating all six alike would keep showing
		// something a person has already dealt with.
		//
		// The reason does not ride along. It is prose one person wrote for
		// another, and a subscriber acting on it is acting on something the
		// author may edit.
		event := crmcontracts.PublicEventForecastExceptionResolved{
			ExceptionId: openapi_types.UUID(exceptionID),
			Outcome:     crmcontracts.PublicEventForecastExceptionResolvedOutcome(in.Outcome),
			ActorUserId: openapi_types.UUID(actor.UserID),
			ExpiresAt:   settled,
		}
		return storekit.EmitEvent(ctx, tx, auditID, actor.UserID, event)
	})
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
		// The system's own. A person claiming it would be saying the condition
		// stopped being true without anything having checked.
		return nil, &values.ParseError{
			Field: slotOutcome, Code: "not_allowed",
			Message: "condition_cleared is recorded by the check itself, not by a person",
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
		return noExpiry, nil
	}

	if !suppresses(in.Outcome) {
		return noExpiry, nil
	}
	// A suppressing answer must say WHY. It hides the finding from the screens
	// a revenue commitment is made from, and the next person to see the number
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
			Field: "expires_at", Code: "out_of_range",
			Message: "an expiry in the past suppresses nothing",
		}
	}
	// Longer than the ceiling is refused rather than silently shortened. A
	// caller who asked for a year and got ninety days without being told would
	// believe the finding stays hidden through the next two quarters.
	if in.ExpiresAt.After(ceiling) {
		return nil, &values.ParseError{
			Field: "expires_at", Code: "out_of_range",
			Message: "a finding may be hidden for at most 90 days: a value that was correct in May is a claim about May",
		}
	}
	return in.ExpiresAt, nil
}
