// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The thread confidentiality engine: the resolver for what a mailbox holds
// until somebody says otherwise.
//
// Under the `classified` posture every captured message is born held, which is
// safe and, on its own, useless — a CRM whose mail nobody but the owner can
// read is a mailbox with extra steps. This engine is what makes that posture
// livable: it reads each thread once and opens the ordinary ones, so what stays
// private is the correspondence that had a reason to.
//
// Exactly one kind opens a thread and every other kind holds it, so a model
// that is wrong, unavailable, or out of budget fails towards privacy. That is
// the whole safety argument, and it is why there is no `default` arm anywhere
// below that could turn an unrecognised answer into an opening one.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// confidentialityClaimSize is how many threads one pass leases at a time.
	confidentialityClaimSize = 8
	// confidentialityRetryBackoff spaces a thread that failed for a reason it
	// may outlive — a provider blip, a validator rejection.
	confidentialityRetryBackoff = 30 * time.Minute
	// confidentialityCatchUpCap bounds one pass so a large backlog drains over
	// several cycles rather than holding one workspace's budget for all of it.
	confidentialityCatchUpCap = 40
	// confidentialityActor is the principal every write of this engine is
	// attributed to. The `agent:` prefix is the contract's grammar, not a
	// description of the process.
	confidentialityActor = "agent:capture_confidentiality_verdict"
	// confidentialityReason is what the ledger records as the authority for a
	// machine answer, distinguishing it from an owner's own decision.
	confidentialityReason = "capture_confidentiality_verdict"
)

// ConfidentialityVerdictEngine drains the thread ledger.
type ConfidentialityVerdictEngine struct {
	pool    *pgxpool.Pool
	threads *capture.ThreadVerdictStore
	// people is here for one reason: a personal verdict has to be able to
	// retract the contact capture already made. Capture decides which records a
	// private thread orphaned; people archives them through its own writer, so
	// the write shape holds. Neither module imports the other.
	people *people.Store
	brain  completer
	log    *slog.Logger
}

// NewConfidentialityVerdictEngine builds the engine over the pool and the model
// lane. A nil brain is a deployment with no local model bound: the engine then
// judges nothing and every thread stays held, which is the correct answer and
// not an error.
func NewConfidentialityVerdictEngine(pool *pgxpool.Pool, brain completer, log *slog.Logger) *ConfidentialityVerdictEngine {
	return &ConfidentialityVerdictEngine{
		pool:    pool,
		threads: capture.NewThreadVerdictStore(InstallationDB(pool)),
		people:  people.NewStore(InstallationDB(pool)),
		brain:   brain,
		log:     log,
	}
}

// CanJudge reports whether a model lane was composed. A deployment without one
// holds every thread and says nothing: there is no owner-facing backlog surface
// yet, so what an owner sees today is mail that stays private. Failing the
// sweep instead would fill the log with an alarm about a configuration somebody
// chose.
func (e *ConfidentialityVerdictEngine) CanJudge() bool { return e.brain != nil }

// workspaceCtx adds this pass's provenance to a context whose WORKSPACE the
// caller already bound. It does not bind the workspace itself: the job layer
// does that from the args' own role declaration, and re-binding here would make
// this a second, independent source of truth for the tenant.
func (e *ConfidentialityVerdictEngine) workspaceCtx(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: confidentialityActor,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// RunWorkspace drains up to maxVerdicts threads in the workspace already bound
// in ctx. A budget stop ends the pass cleanly: what was decided is committed,
// and the rest stays claimable for the next cycle.
func (e *ConfidentialityVerdictEngine) RunWorkspace(ctx context.Context, maxVerdicts int) error {
	if !e.CanJudge() {
		// No model bound. Every thread stays held, which is the answer this
		// deployment gets until an admin binds a lane.
		return nil
	}
	if maxVerdicts <= 0 {
		maxVerdicts = confidentialityCatchUpCap
	}
	if _, ok := principal.WorkspaceID(ctx); !ok {
		// Exported, so a caller other than the worker can reach it: refuse
		// rather than run a pass against a context whose tenant nobody bound.
		return fmt.Errorf("confidentiality: a verdict pass requires a workspace-bound context")
	}
	wsCtx := e.workspaceCtx(ctx)
	resolved := 0
	for resolved < maxVerdicts {
		batch, err := e.threads.ClaimDue(wsCtx, confidentialityClaimSize)
		if err != nil {
			return fmt.Errorf("confidentiality: claiming the thread backlog: %w", err)
		}
		if len(batch) == 0 {
			return nil
		}
		n, err := e.judgeClaimed(wsCtx, batch)
		resolved += n
		if errors.Is(err, ai.ErrBudgetDeferred) {
			// Every thread this pass never reached is refunded: no model saw
			// them, and charging for a budget stop would let two quiet cycles
			// exhaust a thread's allowance and retire it to `unsure` — an
			// infrastructure condition turned into a per-thread terminal answer.
			e.releaseBatch(wsCtx, batch)
			e.log.InfoContext(wsCtx, "confidentiality verdict: budget exhausted, stopping the pass", "resolved", resolved)
			return nil
		}
		if err != nil {
			return fmt.Errorf("confidentiality: draining the thread backlog: %w", err)
		}
	}
	return nil
}

// judgeClaimed judges each claimed thread on its OWN model call, and applies
// each answer on its own transaction. The transaction IS the checkpoint, so a
// budget stop or a crash keeps whatever was already decided.
func (e *ConfidentialityVerdictEngine) judgeClaimed(ctx context.Context, claimed []capture.PendingThread) (int, error) {
	applied := 0
	for _, row := range claimed {
		n, err := e.judgeOne(ctx, row)
		applied += n
		if err != nil {
			outOfBudget := errors.Is(err, ai.ErrBudgetDeferred)
			if deferErr := e.threads.Defer(ctx, row, confidentialityRetryBackoff,
				"the confidentiality verdict could not be completed", outOfBudget); deferErr != nil {
				return applied, deferErr
			}
			if outOfBudget {
				return applied, err
			}
			// Any other fault is a property of THIS thread, whose text an
			// outsider writes, so the attempt is charged: refunding it would
			// make the attempt bound unreachable on exactly the path it exists
			// for, and a message crafted to break the answer would be re-judged
			// forever at one paid call a time.
			e.log.WarnContext(ctx, "confidentiality verdict: judging a thread failed",
				"thread", row.ID.String(), "err", err)
			continue
		}
	}
	return applied, nil
}

// judgeOne asks about ONE thread and applies what comes back.
//
// An OPENING answer below the floor is not believed: the thread resolves to
// `unsure`, which holds. A HOLDING answer needs no floor, because requiring
// confidence to hold would publish exactly the threads the model found hardest.
func (e *ConfidentialityVerdictEngine) judgeOne(ctx context.Context, row capture.PendingThread) (int, error) {
	results, err := e.ask(ctx, row)
	if err != nil {
		return 0, err
	}
	if len(results) != 1 {
		return 0, fmt.Errorf("confidentiality: expected one answer for thread %s", row.ID)
	}
	answer := results[0]
	status, known := statusForConfidentiality(answer.Verdict)
	if !known {
		// Unreachable through the validator, and spelled anyway: an unknown
		// kind must never fall through to an opening status.
		return 0, fmt.Errorf("confidentiality: unknown kind for thread %s", row.ID)
	}
	kind := answer.Verdict
	if status == capture.VerdictCleared && float64(answer.Confidence) < confidentialityFloor {
		// Below the floor the OPENING answer is discarded and the thread holds.
		// The kind is kept as the model gave it, because "the model thought
		// this was ordinary and was not sure" is what a human reviewing the
		// backlog needs to see.
		status = capture.VerdictUnsure
	}
	return e.apply(ctx, row, status, kind, float64(answer.Confidence))
}

// apply writes the answer and recomputes what every affected row's audience
// should now be, in ONE transaction.
//
// The recompute is what turns a verdict into visibility: the ledger row is this
// seat's contribution, and activities.RecomputeAudienceTx derives the row's
// audience across every contributing seat. A verdict written without it would
// be an answer nobody could see the effect of.
func (e *ConfidentialityVerdictEngine) apply(
	ctx context.Context, row capture.PendingThread, status, kind string, confidence float64,
) (int, error) {
	applied := 0
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		seen, err := threadAddressesTx(ctx, tx, row)
		if err != nil {
			return err
		}
		won, err := e.threads.ResolveAs(ctx, tx, row, status, kind, confidence, seen, confidentialityReason)
		if err != nil || !won {
			// Lost the claim race. Somebody else answered this thread, and
			// their answer stands: two workers writing different verdicts over
			// each other is the one thing the claim exists to prevent.
			return err
		}
		applied = 1
		// The verdict has to reach the ROW the derivation reads. activity's
		// audience is derived across every seat's capture_import contribution,
		// so a thread ledger updated on its own is an answer with no effect:
		// the message stays exactly as held as it was born.
		if err := e.threads.RecordOutcomeTx(ctx, tx, row, status, kind); err != nil {
			return err
		}
		// The same answer, applied to the messages of this thread this seat had
		// already imported when it came back. A message that arrives AFTER a
		// verdict inherits it; one that arrived before it used to keep its
		// import posture for good, because the thread's question is answered
		// and the unique ledger row stops a second one being opened. Import
		// order is the accident; the admission rule is unchanged.
		outcome, err := e.threads.RecordOutcomeOnThreadTx(ctx, tx, row.ThreadKey, row.UserID, status, kind, seen)
		if err != nil {
			return err
		}
		if err := e.retractPrivateContactsTx(ctx, tx, row, kind); err != nil {
			return err
		}
		if err := recomputeJudgedMessageTx(ctx, tx, row); err != nil {
			return err
		}
		// Each stamped sibling re-derived over every seat's contribution, so a
		// colleague's mailbox still holding this message keeps holding it.
		for _, id := range outcome.Stamped {
			if err := activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](id)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// The thread key is workspace-internal and already in this workspace's
		// own timeline; the model's answer is not, so the error names what was
		// attempted without echoing content.
		return 0, fmt.Errorf("confidentiality: applying %s to a thread: %w", status, err)
	}
	return applied, nil
}

// threadAddressesTx collects the exact addresses this verdict SAW.
//
// This is what binds a later message to the answer: capture admits an OPENING
// verdict only for a sender in this list, so a reply from an address the
// classifier never looked at re-opens the thread rather than inheriting a
// clearance given for a different conversation.
//
// The parties on the MESSAGE the classifier read, and no others. The claim
// hands it the thread's first message alone, so collecting every address on the
// thread would grant inheritance to senders whose text was never judged — the
// hole ResolveAs's own doc names.
func threadAddressesTx(ctx context.Context, tx pgx.Tx, row capture.PendingThread) ([]string, error) {
	// Scoped to the messages THIS SEAT imported, not to the thread key.
	//
	// A thread key is the sender-controlled References root, in a namespace
	// shared by the whole workspace. Reading addresses by thread key alone lets
	// an outsider send a forged-root message to a colleague while this seat's
	// thread is pending, and have their own address recorded as one this seat's
	// verdict saw — after which a sensitive message from that address inherits
	// the clearance. The import row is the evidence that this seat's mailbox
	// actually received the message.
	if row.ActivityID == ids.Nil {
		return nil, nil
	}
	// restricted_at excluded: a message under a statutory hold or an erasure is
	// not one whose parties a later verdict may inherit from. The claim already
	// refuses to read its text for the same reason.
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT lower(trim(counterparty_email))
		  FROM activity
		 WHERE id = $1 AND restricted_at IS NULL
		   AND counterparty_email IS NOT NULL
		   AND counterparty_email <> ''`, row.ActivityID)
	if err != nil {
		return nil, fmt.Errorf("confidentiality: reading the addresses a thread's verdict saw: %w", err)
	}
	defer rows.Close()
	seen := make([]string, 0, 4)
	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return nil, fmt.Errorf("confidentiality: reading the addresses a thread's verdict saw: %w", err)
		}
		seen = append(seen, address)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("confidentiality: reading the addresses a thread's verdict saw: %w", err)
	}
	return seen, nil
}

// recomputeJudgedMessageTx re-derives the audience of the message this verdict
// was about, so the answer reaches the row it concerns.
//
// One message, matching the stamp above. The thread's other messages were never
// read by the classifier and keep whatever their own contributors ask for.
func recomputeJudgedMessageTx(ctx context.Context, tx pgx.Tx, row capture.PendingThread) error {
	if row.ActivityID == ids.Nil {
		// The message was erased while the question stood. There is nothing to
		// recompute, and the verdict is still worth recording for the threads
		// that inherit from it.
		return nil
	}
	return activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](row.ActivityID))
}

// releaseBatch hands a whole claimed batch back after a budget stop, so no
// thread is charged for a pass that never reached a model.
func (e *ConfidentialityVerdictEngine) releaseBatch(ctx context.Context, batch []capture.PendingThread) {
	for _, row := range batch {
		if err := e.threads.Defer(ctx, row, confidentialityRetryBackoff,
			"the workspace was out of model budget", true); err != nil {
			e.log.WarnContext(ctx, "confidentiality verdict: releasing a claimed thread failed",
				"thread", row.ID.String(), "err", err)
		}
	}
}

// RetireExhausted ends the threads that spent every attempt without an answer.
//
// They resolve to `unsure`, which holds: a thread nothing could judge is
// exactly the one not to publish on a guess. Exported so the job can run it as
// its own stage — a retirement that rode inside the judging pass would be
// skipped on every deployment with no model bound, which is the deployment
// where threads exhaust their attempts.
func (e *ConfidentialityVerdictEngine) RetireExhausted(ctx context.Context) (int64, error) {
	if _, ok := principal.WorkspaceID(ctx); !ok {
		return 0, fmt.Errorf("confidentiality: retiring threads requires a workspace-bound context")
	}
	return e.threads.RetireExhausted(e.workspaceCtx(ctx),
		"no confidentiality answer was reached before the attempts ran out")
}

// confidentialityStragglerBatch bounds one sweep pass. What a pass leaves
// behind the next one picks up: the backlog is a query, so the bound limits the
// work per tick rather than the coverage.
const confidentialityStragglerBatch = 200

// FinishSettledThreads applies each settled question's answer to the messages
// of that thread that never took it.
//
// The verdict path does this as it commits, so this reaches only what that path
// could not: a thread retired without an apply, a thread judged before that
// pass existed, and an apply that lost the claim race after the ledger was
// written. Its subject is a query, so a workspace with none does nothing.
func (e *ConfidentialityVerdictEngine) FinishSettledThreads(ctx context.Context) (int, error) {
	// The pass's own provenance, taken once for the listing and again per
	// thread below, so each repair's stamps and audience events trace together
	// under a correlation id of their own.
	settled, err := e.threads.ThreadsWithUndecidedMessages(
		e.workspaceCtx(ctx), confidentialityStragglerBatch)
	if err != nil {
		return 0, fmt.Errorf("confidentiality: listing settled threads with undecided messages: %w", err)
	}
	finished := 0
	for _, t := range settled {
		done, err := e.finishOneSettledThread(ctx, t)
		if err != nil {
			return finished, err
		}
		if done {
			finished++
		}
	}
	return finished, nil
}

// finishOneSettledThread is one thread's repair, in one transaction.
func (e *ConfidentialityVerdictEngine) finishOneSettledThread(
	ctx context.Context, t capture.SettledThread,
) (bool, error) {
	done := false
	// The pass's own provenance, and every write inside the transaction runs
	// under it: an audit row is written from the actor on the context, so a
	// stage that opened its transaction with one context and wrote with another
	// fails at the first audited write.
	wsCtx := e.workspaceCtx(ctx)
	err := database.WithWorkspaceTx(wsCtx, e.pool, func(tx pgx.Tx) error {
		// Re-read under the lock: the sweep has no claim, so the answer this
		// pass applies must be the one the row still holds rather than the one
		// the listing read.
		locked, ok, err := e.threads.LockSettledThreadTx(wsCtx, tx, t.ThreadKey, t.UserID)
		if err != nil || !ok {
			return err
		}
		outcome, err := e.threads.RecordOutcomeOnThreadTx(
			wsCtx, tx, locked.ThreadKey, locked.UserID, locked.Status, locked.Kind, locked.Seen)
		if err != nil {
			return err
		}
		for _, id := range outcome.Stamped {
			if err := activities.RecomputeAudienceTx(wsCtx, tx, ids.From[ids.ActivityKind](id)); err != nil {
				return err
			}
		}
		done = len(outcome.Stamped) > 0 || outcome.Reopened != ids.Nil
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("confidentiality: finishing a settled thread: %w", err)
	}
	return done, nil
}
