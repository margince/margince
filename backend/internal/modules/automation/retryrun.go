// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// Retrying one failed firing.
//
// A retry re-dispatches the ORIGINAL event down the ordinary runOne path, so
// every gate a first firing passes it passes again: the owner's live RBAC, the
// audience check, the run claim, the effect claim. Nothing here re-applies the
// stored `planned` effect — that was a plan made against a database which has
// since moved, and replaying it would write yesterday's answer over today's
// records.
//
// Three facts must ALL hold, each refusing for its own reason:
//
//  1. The run FAILED. A `blocked` run is the permission gate having refused it
//     on purpose (engine_blocked.go); retrying a refusal asks the same question
//     expecting a different answer, and reads to an operator as though the
//     refusal had been a glitch.
//  2. The handler declares RedrivableWithoutDuplicating (workflow.Spec). Every
//     handler registered today does, each for a reason stated at its own Spec,
//     so this arm refuses nothing now — it is here for the NEXT handler, whose
//     zero value is false until somebody audits it.
//  3. The trigger event is still recoverable. workflow_run keeps the event's id
//     and nothing else about it — no Entity, no Payload — so the envelope has to
//     come back from event_outbox. A CLOCK-triggered run synthesizes a fresh id
//     per evaluation pass (timescan.go) that was never an outbox row, so those
//     are unreplayable by construction rather than by policy: the clock will
//     re-evaluate the same anchor on its own next pass anyway.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// runStatusFailed is the workflow_run.status a retry acts on. The other four
// statuses in the CHECK constraint are each a reason to refuse: applied and
// skipped have nothing to retry, requires_approval is waiting on a human, and
// blocked is the permission gate having already said no.
const runStatusFailed = "failed"

// RetryRefusal names why a failed run may not be re-driven, in the vocabulary
// an operator reads rather than the one the tables use.
type RetryRefusal string

const (
	// RetryRefusedNotFailed covers a run that did not fail: an applied or
	// skipped run has nothing to retry, and a blocked one was refused on
	// purpose.
	RetryRefusedNotFailed RetryRefusal = "not_failed"
	// RetryRefusedRepeats is a handler a second pass would duplicate — the
	// answer Spec.RedrivableWithoutDuplicating carries.
	RetryRefusedRepeats RetryRefusal = "repeats_its_effect"
	// RetryRefusedEventGone is a run whose trigger event cannot be rebuilt: a
	// clock firing, or an outbox row aged out from under it.
	RetryRefusedEventGone RetryRefusal = "trigger_event_unavailable"
)

// RetryOutcome answers what a retry did, or why it did nothing. Refusal is
// empty exactly when Retried is true.
type RetryOutcome struct {
	Retried bool
	Refusal RetryRefusal
}

// retryCandidateSQL recovers everything one re-dispatch needs. The run row
// names the handler, the trigger event and the key; the key's "@<automation
// id>" suffix names the instance, whose own row carries the params and the
// owner this firing acts for. The outbox envelope carries the event itself.
//
// The join to automation is INNER on the live, enabled row for the reason
// troubledRunsSQL's is: a rule its owner turned off must not be re-driven by a
// card left over from before they turned it off. The join to event_outbox is
// LEFT, because a missing envelope is an ANSWER here (refusal 3) rather than a
// reason to report the run as absent.
const retryCandidateSQL = `
WITH target AS (
  SELECT id, status, handler, trigger_event, idempotency_key,
         -- The BASE key: this run's own attempt marker stripped. Retrying a
         -- RETRY must count the attempts the original firing has already had,
         -- so both cases ask the same question. Counting against the unstripped
         -- key restarts at zero on a second retry, which re-mints the first
         -- retry's key, loses the run claim, and returns having applied nothing
         -- while reporting success.
         regexp_replace(idempotency_key, '^retry[0-9]+:', '') AS base_key
    FROM workflow_run WHERE id = $1
)
SELECT t.status, t.handler, a.id, a.params, a.owner_id, o.envelope,
       (SELECT count(*) FROM workflow_run prior
         WHERE prior.handler = t.handler
           AND prior.idempotency_key <> t.base_key
           AND regexp_replace(prior.idempotency_key, '^retry[0-9]+:', '') = t.base_key)
  FROM target t
  JOIN automation a ON a.archived_at IS NULL AND a.enabled
   AND t.handler = a.key
   AND t.idempotency_key LIKE '%@' || a.id
  LEFT JOIN event_outbox o ON o.id = t.trigger_event`

// retryCandidate is one recovered firing, ready to re-dispatch.
type retryCandidate struct {
	handler workflow.Handler
	event   workflow.Event
}

// RetryRun re-dispatches one failed firing and answers what happened. A
// refusal is a value, not an error: "this run may not be retried" is an answer
// about the row, and the caller renders it rather than failing on it.
//
// The caller gates it. This is the engine, which acts as the system principal
// by construction (HandleEvent stamps PrincipalSystem before any handler runs)
// — a system principal bypasses object RBAC entirely, so an authorization
// check placed HERE would be checking the system's authority and would pass for
// everyone. The reader's authority is asked at the HTTP boundary, where a real
// principal still exists.
func (e *WorkflowEngine) RetryRun(ctx context.Context, runID ids.UUID) (RetryOutcome, error) {
	candidate, outcome, err := e.retryCandidateFor(ctx, runID)
	if err != nil || !outcome.Retried {
		return outcome, err
	}
	// The same context shape HandleEvent builds: a retry is the firing
	// happening again, not a new kind of caller.
	runCtx := principal.WithWorkspaceID(ctx, candidate.event.WorkspaceID)
	runCtx = principal.WithActor(runCtx,
		principal.Principal{Type: principal.PrincipalSystem, ID: systemActor})
	runCtx = principal.WithCorrelationID(runCtx, ids.NewV7())
	runCtx = principal.WithCausationEvent(runCtx, candidate.event.ID)
	if err := e.runOne(runCtx, candidate.handler, candidate.event); err != nil {
		return RetryOutcome{}, fmt.Errorf("retrying %s: %w", candidate.handler.Spec().Name, err)
	}
	return outcome, nil
}

// retryCandidateFor reads the run and answers all three questions at once.
func (e *WorkflowEngine) retryCandidateFor(ctx context.Context, runID ids.UUID) (retryCandidate, RetryOutcome, error) {
	ws, err := e.db.Workspace(ctx)
	if err != nil {
		return retryCandidate{}, RetryOutcome{}, err
	}
	var found retryCandidate
	var outcome RetryOutcome
	err = e.db.Tx(ctx, func(tx pgx.Tx) error {
		// One retry decision at a time per run. The attempt count and the run
		// claim it feeds are two transactions — this one reads the count, and
		// runOne's claimRun writes the row — so without serializing them two
		// concurrent retries of one failed run can read different counts,
		// compute different keys, and BOTH dispatch: one failed firing becomes
		// two live ones, and the unique key each claims does not collide, so
		// nothing refuses either. The lock is transaction-scoped and released
		// when this read commits; the loser then reads the winner's count.
		if _, lockErr := tx.Exec(ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended('workflow_run_retry:' || $1::text, 0))`,
			runID); lockErr != nil {
			return lockErr
		}
		var status, handlerName string
		var automationID ids.UUID
		var params json.RawMessage
		var owner *ids.UUID
		var envelope []byte
		var priorAttempts int
		scanErr := tx.QueryRow(ctx, retryCandidateSQL, runID).Scan(
			&status, &handlerName, &automationID, &params, &owner, &envelope, &priorAttempts)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if scanErr != nil {
			return scanErr
		}
		if status != runStatusFailed {
			outcome = RetryOutcome{Refusal: RetryRefusedNotFailed}
			return nil
		}
		handler := e.handlerNamed(handlerName)
		if handler == nil || !handler.Spec().RedrivableWithoutDuplicating {
			// An unregistered handler answers the same way an unaudited one
			// does: nobody can say a second pass is safe, so nobody may.
			outcome = RetryOutcome{Refusal: RetryRefusedRepeats}
			return nil
		}
		if len(envelope) == 0 {
			outcome = RetryOutcome{Refusal: RetryRefusedEventGone}
			return nil
		}
		event, decodeErr := eventFromEnvelope(envelope, ws.UUID)
		if decodeErr != nil {
			return decodeErr
		}
		event.AutomationID = automationID
		event.Params = params
		// The attempt AFTER the ones already recorded, so a second retry of the
		// same firing claims its own run row rather than folding into the first
		// retry's. Counting rows rather than tracking a counter keeps the two
		// from drifting: the run rows ARE the record of what has been tried.
		event.RetryAttempt = priorAttempts + 1
		if owner != nil {
			event.OwnerID = *owner
		}
		found = retryCandidate{handler: handler, event: event}
		outcome = RetryOutcome{Retried: true}
		return nil
	})
	if err != nil {
		return retryCandidate{}, RetryOutcome{}, fmt.Errorf("reading the run to retry: %w", err)
	}
	return found, outcome, nil
}

// handlerNamed finds one registered handler by its spec name, under the same
// lock discipline dispatch reads the registry with, and returns nil for a name
// nothing registers. Both registries are searched: a system handler fails and
// is retried exactly like an instance one.
//
//nolint:ireturn // the registry holds workflow.Handler and the engine deliberately does not know the implementations; a concrete return type would name one.
func (e *WorkflowEngine) handlerNamed(name string) workflow.Handler {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, h := range append(append([]workflow.Handler(nil), e.handlers...), e.system...) {
		if h.Spec().Name == name {
			return h
		}
	}
	return nil
}

// eventFromEnvelope rebuilds the event a stored outbox row carried, in the same
// shape HandleEvent builds one from a live delivery — including the workspace,
// which the envelope does not carry (the bus is untenanted, ADR-0091 §6) and
// which is this engine's own.
func eventFromEnvelope(raw []byte, workspace ids.UUID) (workflow.Event, error) {
	var env kevents.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return workflow.Event{}, fmt.Errorf("decoding the stored trigger event: %w", err)
	}
	return workflow.Event{
		ID:          env.EventID,
		Type:        env.Type,
		WorkspaceID: workspace,
		OccurredAt:  env.OccurredAt,
		Entity: datasource.EntityRef{
			Type: datasource.EntityType(env.Entity.Type),
			ID:   env.Entity.ID,
		},
		Payload: env.Payload,
	}, nil
}
