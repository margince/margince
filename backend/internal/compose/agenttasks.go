// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The durable half of the MCP Tasks extension: the agent_task rows behind the
// handle a staged confirm-first call hands back.
//
// The store lives here rather than in modules/agents for the reason the
// idempotency claim does — that package declares seams and owns no SQL — and a
// task is operational state of the same class as the claim: it records what a
// surface did, never what the business knows. So no audit row, no outbox row.
// The effect a task performs is executed through Registry.Invoke and carries
// the full write shape there.
//
// Every query is scoped to the calling agent, in the SQL rather than by a
// caller remembering to filter. A task belonging to another agent is absent,
// not forbidden: distinguishing the two would tell a caller which ids exist.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// agentTasks implements the tool surface's task seam.
type agentTasks struct{ pool *pgxpool.Pool }

var _ agents.Tasks = agentTasks{}

// toolTasks is the task store every composed tool surface installs.
func toolTasks(pool *pgxpool.Pool) agentTasks { return agentTasks{pool: pool} }

// taskColumns is the projection every read shares, so three queries cannot
// disagree about what a task is.
const taskColumns = `id, approval_id, tool, status, COALESCE(status_message, ''), result, error,
	served_records, expires_at, created_at, updated_at`

// taskOwnedBy scopes every query to the agent: $2 is its passport and $3 its
// connection, which a token refresh keeps while it retires the passport.
const taskOwnedBy = `(passport_id = $2 OR connection_id = $3)`

// taskOwnerOf answers the caller's passport and connection; a nil one is NULL.
//
// A zero passport is refused as hard as a missing principal. principal.Actor
// answers ok for a zero-value Principal. A passport-less task row would be one
// row every such caller shares. A task id is useless to all but its owner.
func taskOwnerOf(ctx context.Context, verb string) (ids.UUID, *ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.PassportID.IsZero() {
		return ids.Nil, nil, fmt.Errorf("compose: no agent passport %s an MCP task", verb)
	}
	if actor.ConnectionID.IsZero() {
		return actor.PassportID, nil, nil
	}
	return actor.PassportID, &actor.ConnectionID, nil
}

// Create records the handle. It commits before answering, which is the
// specification's own requirement: a client must never have to poll
// speculatively for its own task to appear.
//
// ONE handle per approval, which is what idx_agent_task_approval already says —
// so a second call arriving at the same approval is answered with the handle
// that exists rather than failing on the index. It reaches here because a
// repeated 🟡 call is now answered with the approval already on the table
// (approvals.StageAgentCall) instead of a fresh one, and there is exactly one
// right answer for the agent: the task tracking the decision it is waiting for.
// Minting a second handle would give one question two answers, and letting the
// insert fail would drop the agent back to a bare refusal and lose the handle it
// had.
//
// Only a LIVE row is answered, and the existing one is returned UNCHANGED. A
// terminal row is not an answer to a live approval: an agent that cancels its
// task leaves the approval approved and unspent — Withdraw is a no-op against a
// decided one — so within the redemption window the identical call is handed that
// same approval, and answering it with a "cancelled" handle would hide a decision
// the agent can still spend behind a dead one, for every re-issue until the window
// closed. A terminal row therefore fails the predicate and the caller gets the
// plain refusal, which names the approval and the header that redeems it.
func (t agentTasks) Create(ctx context.Context, in agents.NewTask) (agents.Task, error) {
	passport, connection, err := taskOwnerOf(ctx, "creating")
	if err != nil {
		return agents.Task{}, err
	}
	var task agents.Task
	err = database.WithWorkspaceTx(ctx, t.pool, func(tx pgx.Tx) error {
		// DO UPDATE over DO NOTHING, because DO NOTHING returns no row at all and
		// this needs the existing one. Touching a column it already holds is what
		// makes RETURNING fire; approval_id is the conflict key, so the write is a
		// no-op by construction.
		//
		// The WHERE carries both conditions on answering an existing row. The
		// owner half is here rather than trusted upstream, so a handle is answered
		// only to the agent that holds it (taskOwnerOf). Another agent's row fails
		// the predicate, RETURNING yields nothing, and the caller gets the plain
		// refusal. The status half keeps a terminal row from standing in for a
		// live decision (see above). Either way the no-row path is the fallback.
		row := tx.QueryRow(ctx, `
			INSERT INTO agent_task (approval_id, passport_id, connection_id, tool, status_message, expires_at)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
			ON CONFLICT (approval_id) DO UPDATE SET approval_id = agent_task.approval_id
			  WHERE (agent_task.passport_id = $2 OR agent_task.connection_id = $3) AND agent_task.status = $7
			RETURNING `+taskColumns,
			in.ApprovalID, passport, connection, in.Tool, in.StatusMessage, in.ExpiresAt, agents.TaskWorking)
		return scanTask(row, &task)
	})
	if err != nil {
		return agents.Task{}, fmt.Errorf("compose: creating an MCP task: %w", err)
	}
	return task, nil
}

// Load answers this agent's live task. The specification permits discarding an
// expired one, so it reads as absent rather than as a task nobody can use.
func (t agentTasks) Load(ctx context.Context, id ids.UUID) (agents.Task, error) {
	passport, connection, err := taskOwnerOf(ctx, "reading")
	if err != nil {
		return agents.Task{}, err
	}
	var task agents.Task
	err = database.WithWorkspaceTx(ctx, t.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+taskColumns+`
			FROM agent_task WHERE id = $1 AND `+taskOwnedBy+` AND expires_at > now()`, id, passport, connection)
		return scanTask(row, &task)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return agents.Task{}, apperrors.ErrNotFound
	}
	if err != nil {
		return agents.Task{}, fmt.Errorf("compose: reading an MCP task: %w", err)
	}
	return task, nil
}

// Claim takes a working task for execution.
//
// The UPDATE is the whole concurrency argument: two polls racing here produce
// one row affected and one zero, and the loser reports `working` rather than
// executing a second time. `reclaimed` is the row's own memory that somebody
// held this claim before and never settled — the only local evidence that an
// execution died mid-flight, since the approval cannot tell a poll that crashed
// from a direct call that redeemed it.
func (t agentTasks) Claim(ctx context.Context, id ids.UUID, lease time.Duration) (agents.TaskClaim, error) {
	passport, connection, err := taskOwnerOf(ctx, "claiming")
	if err != nil {
		return agents.TaskClaim{}, err
	}
	var claim agents.TaskClaim
	err = database.WithWorkspaceTx(ctx, t.pool, func(tx pgx.Tx) error {
		// `prior` reads the row as it was BEFORE the update, because every CTE
		// in one statement sees the same snapshot. That is the only way to learn
		// what claimed_at held: RETURNING answers the new value, which this
		// statement has just overwritten.
		return tx.QueryRow(ctx, `
			WITH prior AS (
				SELECT claimed_at FROM agent_task WHERE id = $1 AND `+taskOwnedBy+`
			), taken AS (
				UPDATE agent_task SET claimed_at = now(), updated_at = now()
				WHERE id = $1 AND `+taskOwnedBy+` AND status = 'working' AND expires_at > now()
				  AND (claimed_at IS NULL OR claimed_at < now() - $4::interval)
				RETURNING id
			)
			SELECT EXISTS (SELECT 1 FROM taken),
			       COALESCE((SELECT claimed_at IS NOT NULL FROM prior), false)`,
			id, passport, connection, lease).Scan(&claim.Won, &claim.Reclaimed)
	})
	if err != nil {
		return agents.TaskClaim{}, fmt.Errorf("compose: claiming an MCP task: %w", err)
	}
	// A claim not won says nothing about who held it: another poll is executing
	// right now, which is not the interrupted case and must not be reported as
	// one.
	if !claim.Won {
		return agents.TaskClaim{}, nil
	}
	return claim, nil
}

// Settle records a terminal state, and answers the task as it now stands.
//
// The status guard is what keeps a terminal state immutable. A settle arriving
// for an already-settled task changes nothing and returns what is stored, so
// the first answer a client was given stays the answer.
func (t agentTasks) Settle(ctx context.Context, id ids.UUID, s agents.Settlement) (agents.Task, error) {
	passport, connection, err := taskOwnerOf(ctx, "settling")
	if err != nil {
		return agents.Task{}, err
	}
	var task agents.Task
	err = database.WithWorkspaceTx(ctx, t.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE agent_task SET status = $4, status_message = NULLIF($5, ''),
				result = $6, error = $7, served_records = $8, updated_at = now()
			WHERE id = $1 AND `+taskOwnedBy+` AND status = 'working'
			RETURNING `+taskColumns,
			id, passport, connection, string(s.Status), s.StatusMessage,
			nullableJSON(s.Result), nullableJSON(s.Error), s.ServedRecords)
		if scanErr := scanTask(row, &task); !errors.Is(scanErr, pgx.ErrNoRows) {
			return scanErr
		}
		// Already terminal. The specification makes that state immutable, so
		// what is stored IS the answer — including for the caller that just
		// tried to overwrite it.
		return scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+`
			FROM agent_task WHERE id = $1 AND `+taskOwnedBy, id, passport, connection), &task)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return agents.Task{}, apperrors.ErrNotFound
	}
	if err != nil {
		return agents.Task{}, fmt.Errorf("compose: settling an MCP task: %w", err)
	}
	return task, nil
}

// Cancel settles a task NOTHING IS EXECUTING.
//
// The claim guard is the whole of it, and it is what a status check alone could
// not give: a task inside its released call would otherwise be marked cancelled,
// discarding that call's own settlement and telling the client nothing happened
// while the effect was committing.
//
// It is the SAME predicate Claim and the retention sweep use — a live claim is
// one younger than the executor's lease — because all three are asking one
// question, and a cancel with a stricter answer would leave a task whose
// executor died permanently uncancellable. So losing here means an executor
// really is inside the row, and that poll will settle the task itself:
// cancellation is cooperative, and the specification permits a terminal state
// other than cancelled.
func (t agentTasks) Cancel(ctx context.Context, id ids.UUID, lease time.Duration, message string) (bool, error) {
	passport, connection, err := taskOwnerOf(ctx, "cancelling")
	if err != nil {
		return false, err
	}
	var cancelled bool
	err = database.WithWorkspaceTx(ctx, t.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			WITH taken AS (
				UPDATE agent_task SET status = 'cancelled', status_message = NULLIF($5, ''), updated_at = now()
				WHERE id = $1 AND `+taskOwnedBy+` AND status = 'working'
				  AND (claimed_at IS NULL OR claimed_at < now() - $4::interval)
				RETURNING id
			)
			SELECT EXISTS (SELECT 1 FROM taken)`, id, passport, connection, lease, message).Scan(&cancelled)
	})
	if err != nil {
		return false, fmt.Errorf("compose: cancelling an MCP task: %w", err)
	}
	return cancelled, nil
}

// nullableJSON writes an absent payload as SQL NULL rather than as the two
// bytes `null`, which the terminal-payload CHECK reads as a value that is
// present. A cancelled task carrying `result = 'null'::jsonb` would fail the
// constraint, and it would fail it at the moment a human's cancellation was
// being recorded.
func nullableJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func scanTask(row pgx.Row, into *agents.Task) error {
	var (
		approvalID ids.UUID
		status     string
		result     []byte
		failure    []byte
	)
	if err := row.Scan(&into.ID, &approvalID, &into.Tool, &status, &into.StatusMessage,
		&result, &failure, &into.ServedRecords, &into.ExpiresAt, &into.CreatedAt, &into.UpdatedAt); err != nil {
		return err
	}
	into.ApprovalID = ids.From[ids.ApprovalKind](approvalID)
	into.Status = agents.TaskStatus(status)
	into.Result, into.Error = json.RawMessage(result), json.RawMessage(failure)
	return nil
}
