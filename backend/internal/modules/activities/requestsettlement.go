// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether our own reply settled the request an inbound message made.
//
// owed_verdict answers a question about ONE message: does it ask the recipient
// side for something. It is written once, from that message alone, and nothing
// revisits it — which is right for what it says and wrong for what a reader
// then sees, because a request answered within the hour still reads as owed a
// week later. The missing half is not a better verdict on the inbound mail. It
// is a second question, asked of the THREAD, and only askable once we have
// written back.
//
// TWO THINGS THIS DELIBERATELY DOES NOT DO.
//
// It does not settle on the existence of a reply. "Thanks, I will check" is a
// reply and discharges nothing, and the two integration tests that pin that
// behaviour stay green under this file: the judgement is a model's, and with no
// model configured nothing here runs at all. What changes is that a reply is
// now EVIDENCE the question is asked over, where before it was invisible.
//
// It does not reopen. A settled request whose task a human has reopened is
// theirs again — ApplySettlement skips a task that moved underneath it rather
// than retrying, which is the opposite of the follow-up completer beside it and
// is the whole difference between a machine finishing work and a machine
// overruling somebody.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The closed set the column's CHECK admits, spelled here because the store is
// what writes it and a third spelling would be refused by the database after a
// model call has already been paid for.
const (
	// RequestSettled: our words answered, declined, delivered, booked, or
	// handed the ask to somebody else. Nothing is left for us to do.
	RequestSettled = "settled"
	// RequestStillOwed: we replied and left something undone. Remaining names
	// it in the reader's own seat.
	RequestStillOwed = "still_owed"
	// RequestUnsure: the model would not commit above the floor. A real answer
	// and not an absence — the request stays owed, which is what it was before
	// this pass existed, and the watermark advances so the same thread is not
	// re-asked until somebody writes again.
	RequestUnsure = "unsure"
)

// The settlement columns. Both the audit images and the read-back spell them,
// and a drifting pair would put one column's value under another's name in
// audit_log.
const (
	settlementVerdictCol   = "verdict"
	settlementThroughCol   = "judged_through_activity_id"
	settlementRemainingCol = "remaining"
	settlementDueCol       = "due_at"
	settlementConfidence   = "confidence"
	settlementDecidedByCol = "decided_by"
)

// OwedVerdictCapturedBy is what the owed pass stamps on a reminder it filed.
//
// Declared HERE, in the module that writes the row, and read by the compose
// engine that mints the principal — rather than the other way round, which the
// layering forbids anyway. The string decides whether a task may be retitled by
// a machine, so a second spelling of it would be a task nobody could tell from
// a human's own work.
const OwedVerdictCapturedBy = "system:owed_verdict"

// RepliedRequest is one request the workspace has since written back on, as the
// settlement pass consumes it.
type RepliedRequest struct {
	RequestID ids.UUID
	// NewestOutboundID is the reply the judgement is reached over, and the
	// watermark written beside the verdict. A later one re-arms the question.
	NewestOutboundID ids.UUID
	// TaskID is the email_request reminder minted from this request, when one
	// exists. A request outside the assignment horizon has none, and then a
	// verdict is recorded and nothing is completed.
	TaskID *ids.UUID
	// TaskVersion is what the selection read, carried so the effect can refuse
	// a task somebody edited in between.
	TaskVersion int64
	// TaskMachineMinted says the reminder is the one the owed pass filed, never
	// one a human accepted. Only a machine-minted, undated task may be retitled
	// — a human's own subject and deadline are theirs.
	TaskMachineMinted bool
	// CounterpartyEmail is who this request was with. Carried out of the
	// selection so the conversation read can bind to the same correspondent
	// this statement matched on, rather than reading the column a second time:
	// the two statements run under read-committed, and a value that moved in
	// between would widen the window the selection had already narrowed.
	CounterpartyEmail string
}

// repliedRequestsSQL selects requests the workspace has answered since.
//
// The reply test is the ordinary thread comparison — same key, same kind, same
// provider — bounded at asOf so a SCHEDULED send cannot settle a request before
// it has left the building.
//
// It is ALSO bound to one correspondent, and the thread triple is not enough to
// do that. thread_key can be the RFC822 References root, which the SENDER types:
// a stranger who has seen one of our Message-IDs can forge a thread onto an
// unrelated customer's conversation. Matching outbound on the key alone then
// reads our reply to THEM as a reply to the stranger. counterparty_email names
// who the message was actually with, and counterparty_outbound_attested is the
// provider's own filing of it as sent there — a header cannot forge either. That bound is the one waitingengagement's own tests
// hold, and for the same reason: a future-dated row is not something the
// customer has received.
//
// The watermark arm is what makes the pass idempotent and re-armable at once. A
// thread already judged through its newest outbound is skipped; one that has
// since been written on again comes back, because the newest outbound is no
// longer the one the row names.
const repliedRequestsSQL = outstandingRequestSQL + `
 AND a.audience = 'workspace'
 AND a.counterparty_email IS NOT NULL
 AND EXISTS (SELECT 1 FROM activity reply
   WHERE reply.thread_key = a.thread_key AND reply.kind = a.kind
     AND reply.channel_provider IS NOT DISTINCT FROM a.channel_provider
     AND reply.direction = 'outbound' AND reply.archived_at IS NULL
     AND reply.counterparty_email = a.counterparty_email
     AND reply.counterparty_outbound_attested
     AND reply.occurred_at <= $1
     AND (reply.occurred_at, reply.id) > (a.occurred_at, a.id))
 AND NOT EXISTS (SELECT 1 FROM activity_request_settlement judged
   WHERE judged.request_activity_id = a.id
     AND judged.judged_through_activity_id = (
       SELECT newest.id FROM activity newest
        WHERE newest.thread_key = a.thread_key AND newest.kind = a.kind
          AND newest.channel_provider IS NOT DISTINCT FROM a.channel_provider
          AND newest.direction = 'outbound' AND newest.archived_at IS NULL
          AND newest.counterparty_email = a.counterparty_email
          AND newest.counterparty_outbound_attested
          AND newest.occurred_at <= $1
          AND (newest.occurred_at, newest.id) > (a.occurred_at, a.id)
        ORDER BY newest.occurred_at DESC, newest.id DESC LIMIT 1))`

// RepliedRequests reads the requests this workspace has answered and not yet
// judged, oldest first.
//
// System principal only, like the pass that mints the tasks: this hands thread
// text to a model, and the audience clause is what decides that a conversation
// the confidentiality engine narrowed never leaves the building.
func (s *Store) RepliedRequests(ctx context.Context, asOf time.Time, limit int) ([]RepliedRequest, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return nil, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []RepliedRequest
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, storekit.SQLf(`
			SELECT a.id, coalesce(a.counterparty_email, ''),
			       (SELECT newest.id FROM activity newest
			         WHERE newest.thread_key = a.thread_key AND newest.kind = a.kind
			           AND newest.channel_provider IS NOT DISTINCT FROM a.channel_provider
			           AND newest.direction = 'outbound' AND newest.archived_at IS NULL
			           AND newest.counterparty_email = a.counterparty_email
			           AND newest.counterparty_outbound_attested
			           AND newest.occurred_at <= $1
			           AND (newest.occurred_at, newest.id) > (a.occurred_at, a.id)
			         ORDER BY newest.occurred_at DESC, newest.id DESC LIMIT 1),
			       task.id, coalesce(task.version, 0),
			       -- Whether the reminder is still the machine's to sharpen: the
			       -- pass filed it, nobody has dated it, and nobody has renamed
			       -- it. The SUBJECT comparison is the half captured_by cannot
			       -- carry — a human renaming a task leaves the stamp alone, so
			       -- without it the machine could overwrite the title somebody
			       -- had just typed.
			       coalesce(task.captured_by = $2 AND task.due_at IS NULL
			                AND task.subject IS NOT DISTINCT FROM a.subject, false)
			  FROM activity a
			  LEFT JOIN activity task
			    ON task.source_system = '%s' AND task.source_activity_id = a.id
			   AND task.archived_at IS NULL AND task.is_done = false
			 WHERE %s
			 ORDER BY a.occurred_at, a.id
			 LIMIT $3`, EmailRequestTaskSource, repliedRequestsSQL),
			asOf, OwedVerdictCapturedBy, limit)
		if err != nil {
			return fmt.Errorf("activities: reading answered requests: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r RepliedRequest
			if err := rows.Scan(&r.RequestID, &r.CounterpartyEmail, &r.NewestOutboundID,
				&r.TaskID, &r.TaskVersion, &r.TaskMachineMinted); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RequestSettlementInput is one judgement and what it asks of the task.
type RequestSettlementInput struct {
	Request RepliedRequest
	Verdict string
	// Remaining is what is still owed, in the reader's own seat. Only a
	// still_owed verdict carries one; the column's CHECK refuses the rest.
	Remaining string
	DueAt     *time.Time
	// Confidence is the model's own, recorded so a later reader can tell a
	// judgement the pass was sure of from one it barely reached.
	Confidence float64
	// DecidedBy names the classifier that reached it.
	DecidedBy string
}

// SettleRequest records one judgement AND applies it, in one transaction.
//
// The two halves commit together on purpose. Recorded first and applied
// afterwards, a crash between them leaves a watermark saying the question is
// answered over a task nobody finished, and the next pass skips it forever.
// Applied first, a crash leaves the task done and the question unanswered, so
// the next pass asks again and pays for a model call to reach the same verdict.
// One transaction has neither failure.
//
// A verdict is ALWAYS recorded; what varies is whether a task moves. That is
// what makes unsure a real answer rather than a retry: the row advances the
// watermark and nothing else happens.
func (s *Store) SettleRequest(ctx context.Context, in RequestSettlementInput) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return apperrors.ErrPermissionDenied
	}
	if in.Verdict != RequestSettled && in.Verdict != RequestStillOwed && in.Verdict != RequestUnsure {
		return fmt.Errorf("activities: %q is not a settlement this column accepts", in.Verdict)
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		still, err := requestStillWritable(ctx, tx, in.Request.RequestID)
		if err != nil {
			return err
		}
		if !still {
			// Narrowed, archived, held or erased since the candidate read. The
			// pass reads a batch, spends a model call per conversation and
			// writes the answers back, so a human or a privacy verdict can move
			// the row inside that window — and `remaining` is model prose about
			// a customer, which must not land on a message the workspace may no
			// longer open. Not an error: the judgement is simply dropped, and
			// the next pass reads whatever the row says then.
			return nil
		}
		if err := recordSettlement(ctx, tx, in); err != nil {
			return err
		}
		return applySettlement(ctx, tx, in)
	})
}

// requestStillWritable re-tests, at WRITE time, that the request is one this
// pass may still record a judgement about.
//
// The same re-test SetOwedVerdict carries in its own UPDATE's WHERE clause, and
// for the same reason — except that this write lands in a different table, so
// there is no row of the request's own to attach the condition to and it has to
// be asked outright. Without it a settlement that finished after an erasure
// would reinsert a sentence about a subject nobody may now assert anything
// about, into a table the cascade has already swept.
func requestStillWritable(ctx context.Context, tx pgx.Tx, requestID ids.UUID) (bool, error) {
	var live bool
	err := tx.QueryRow(ctx, `
		SELECT true FROM activity
		 WHERE id = $1 AND archived_at IS NULL
		   AND audience = 'workspace' AND restricted_at IS NULL`, requestID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: re-reading the request before settling it: %w", err)
	}
	return live, nil
}

// recordSettlement writes the verdict, replacing any earlier one.
//
// Audited WITH A BEFORE-IMAGE, for the reason every replacing write in this
// tree carries one: a later reply is new evidence about the same question, so
// the row moves, and what it said before is what audit_log is for. SetOwedVerdict
// beside it images its prior state too, since a verdict reached under older
// rules can now be replaced by one reached under newer.
func recordSettlement(ctx context.Context, tx pgx.Tx, in RequestSettlementInput) error {
	before, err := settlementImage(ctx, tx, in.Request.RequestID)
	if err != nil {
		return err
	}
	remaining, dueAt := settlementProse(in)
	after := map[string]any{
		settlementVerdictCol:   in.Verdict,
		settlementThroughCol:   in.Request.NewestOutboundID.String(),
		settlementRemainingCol: remaining,
		settlementDueCol:       dueAt,
		settlementConfidence:   in.Confidence,
		settlementDecidedByCol: in.DecidedBy,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_request_settlement (request_activity_id,
		       judged_through_activity_id, verdict, remaining, due_at, confidence, decided_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (request_activity_id) DO UPDATE SET
		       judged_through_activity_id = EXCLUDED.judged_through_activity_id,
		       verdict = EXCLUDED.verdict, remaining = EXCLUDED.remaining,
		       due_at = EXCLUDED.due_at, confidence = EXCLUDED.confidence,
		       decided_by = EXCLUDED.decided_by, decided_at = now()`,
		in.Request.RequestID, in.Request.NewestOutboundID, in.Verdict,
		remaining, dueAt, in.Confidence, in.DecidedBy); err != nil {
		return fmt.Errorf("activities: recording the request settlement: %w", err)
	}
	// TWO call sites with LITERAL verbs rather than one with a variable, and
	// the gate that asks for it is right to: a verb built at runtime is one no
	// census can judge, so the before-image rule — which binds updates and not
	// creates — could not be checked here at all. Spelled out, each site says
	// what it is. A first judgement CREATES and has no prior state to image; a
	// later one REPLACES, and carries what the row said before.
	if before == nil {
		if _, err := storekit.Audit(ctx, tx, "create", "activity",
			in.Request.RequestID, nil, after); err != nil {
			return err
		}
		return nil
	}
	if _, err := storekit.Audit(ctx, tx, "update", "activity",
		in.Request.RequestID, before, after); err != nil {
		return err
	}
	return nil
}

// settlementProse is the prose half of a verdict, which only still_owed has.
//
// Returned as the pair rather than read twice, because the column's CHECK binds
// them together: a settled or unsure row carries neither, and letting one
// through would be refused by the database with the model call already paid for.
func settlementProse(in RequestSettlementInput) (remaining *string, dueAt *time.Time) {
	if in.Verdict != RequestStillOwed || in.Remaining == "" {
		return nil, nil
	}
	return &in.Remaining, in.DueAt
}

// settlementImage reads what the row says now, for the audit before-image.
//
// A nil map with a nil error is this function's answer for "no prior
// judgement", which is the ordinary case on a first settlement and is exactly
// what the audit door wants: AuditWithEvidence takes a nil before-image as
// "there was no prior state", and a sentinel here would make every first write
// an error path the caller has to recognise and discard.
//
//nolint:nilnil // an absent before-image IS the answer for a first judgement; see above.
func settlementImage(ctx context.Context, tx pgx.Tx, requestID ids.UUID) (map[string]any, error) {
	var verdict, decidedBy string
	var remaining *string
	var judgedThrough ids.UUID
	var dueAt *time.Time
	var confidence float64
	err := tx.QueryRow(ctx, `
		SELECT verdict, judged_through_activity_id, remaining, due_at, confidence, decided_by
		  FROM activity_request_settlement WHERE request_activity_id = $1`,
		requestID).Scan(&verdict, &judgedThrough, &remaining, &dueAt, &confidence, &decidedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("activities: reading the prior settlement: %w", err)
	}
	return map[string]any{
		settlementVerdictCol:   verdict,
		settlementThroughCol:   judgedThrough.String(),
		settlementRemainingCol: remaining,
		settlementDueCol:       dueAt,
		settlementConfidence:   confidence,
		settlementDecidedByCol: decidedBy,
	}, nil
}

// applySettlement does what the verdict asks of the request's own task.
//
// Through updateActivityInTx — the module's own write path — so a completion
// carries the audit row and the activity.updated event a human ticking the box
// carries. A bulk UPDATE here would be invisible history on somebody's task.
//
// VERSION SKEW IS A SKIP, not a retry, and that is the whole safety of the
// feature. completeSystemTask beside this re-reads and tries again, which is
// right for a follow-up loop nobody else is touching; here a moved version
// means a HUMAN edited the task between the selection and now — reopened it,
// renamed it, gave it a date — and the next pass will read their version and
// judge again. Retrying would overwrite the human this feature exists to serve.
func applySettlement(ctx context.Context, tx pgx.Tx, in RequestSettlementInput) error {
	if in.Request.TaskID == nil {
		return nil
	}
	patch, ok := settlementPatch(in)
	if !ok {
		return nil
	}
	version := in.Request.TaskVersion
	patch.IfVersion = &version
	id := ids.From[ids.ActivityKind](*in.Request.TaskID)
	switch _, err := updateActivityInTx(ctx, tx, id, patch); {
	case err == nil, errors.Is(err, apperrors.ErrVersionSkew), errors.Is(err, apperrors.ErrNotFound):
		// Skew: somebody edited the task, and theirs stands. Not found: it was
		// archived between the read and the write. Neither is this pass's
		// business to correct, and both are answered by judging again later.
		return nil
	default:
		return fmt.Errorf("activities: applying the request settlement: %w", err)
	}
}

// settlementPatch is what a verdict asks of the task, or false for nothing.
//
// still_owed sharpens a task the MACHINE filed and nobody has dated: the
// subject becomes what is actually owed rather than the mail's subject line,
// and a deadline lands only if our own words named one. A task a human accepted
// keeps their wording and their date — TaskMachineMinted is what tells the two
// apart, and it is read from captured_by rather than inferred from the subject,
// because a human accepting a request uses the same builder and may keep the
// same title.
func settlementPatch(in RequestSettlementInput) (UpdateActivityInput, bool) {
	switch in.Verdict {
	case RequestSettled:
		done := true
		return UpdateActivityInput{IsDone: &done}, true
	case RequestStillOwed:
		if !in.Request.TaskMachineMinted || in.Remaining == "" {
			return UpdateActivityInput{}, false
		}
		remaining := in.Remaining
		return UpdateActivityInput{Subject: &remaining, DueAt: in.DueAt}, true
	default:
		return UpdateActivityInput{}, false
	}
}
