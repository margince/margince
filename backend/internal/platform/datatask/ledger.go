// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Ledger statuses distinguish unfinished work, operator holds and completed steps.
const (
	Pending = "pending"
	Running = "running"
	Paused  = "paused"
	Done    = "done"
	Failed  = "failed"
)

// Progress distinguishes an unknown estimate from a known empty selection.
type Progress struct {
	TaskID               string
	Title                string
	Step                 string
	RunID                ids.UUID
	TaskVersion          int
	Status               string
	Cursor               json.RawMessage
	BatchLimit           int
	Scanned              int64
	Changed              int64
	Batches              int64
	Estimate             *int64
	EstimateCapped       bool
	EstimatedAt          *time.Time
	ConsecutiveFailures  int
	LastErrorClass       *string
	NextAttemptAt        *time.Time
	LastProgressAt       *time.Time
	FirstRelease         string
	Version              int64
	BlockedReason        string
	BlockedBy            string
	MissingPrerequisites []string
	Activation           *Activation
}

// Engine shares one ledger between worker execution and operator controls.
type Engine struct {
	pool         *pgxpool.Pool
	tasks        []Task
	now          func() time.Time
	release      string
	notifyFailed func(context.Context, pgx.Tx, Progress) error
	beforeCommit func(context.Context, pgx.Tx) error
}

// New rejects incomplete registration before any progress is written.
func New(pool *pgxpool.Pool, tasks []Task, release string, now func() time.Time,
	notifyFailed func(context.Context, pgx.Tx, Progress) error,
) (*Engine, error) {
	if pool == nil || now == nil || notifyFailed == nil {
		return nil, errors.New("data task executor requires a database, clock and failure notice writer")
	}
	catalog, err := Catalog(tasks)
	if err != nil {
		return nil, err
	}
	return &Engine{pool: pool, tasks: catalog, release: release, now: now, notifyFailed: notifyFailed}, nil
}

const progressColumns = `task_id, step, run_id, task_version, status, cursor, batch_limit,
 scanned, changed, batches, estimate, estimate_capped, estimated_at, consecutive_failures,
 last_error_class, next_attempt_at, last_progress_at, first_release, version, activation`

func scanProgress(row pgx.Row) (Progress, error) {
	var p Progress
	err := row.Scan(&p.TaskID, &p.Step, &p.RunID, &p.TaskVersion, &p.Status, &p.Cursor, &p.BatchLimit,
		&p.Scanned, &p.Changed, &p.Batches, &p.Estimate, &p.EstimateCapped, &p.EstimatedAt,
		&p.ConsecutiveFailures, &p.LastErrorClass, &p.NextAttemptAt, &p.LastProgressAt, &p.FirstRelease, &p.Version, &p.Activation)
	return p, err
}

func readProgress(ctx context.Context, tx pgx.Tx) (map[string]Progress, error) {
	rows, err := tx.Query(ctx, `SELECT `+progressColumns+` FROM data_task_run`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]Progress)
	for rows.Next() {
		p, err := scanProgress(rows)
		if err != nil {
			return nil, err
		}
		out[progressKey(p.TaskID, p.Step)] = p
	}
	return out, rows.Err()
}

func progressKey(task, step string) string { return task + ":" + step }

// Seed is replay-safe; only the worker calls it, never API startup.
func (e *Engine) Seed(ctx context.Context) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, transactionLimit)
	defer cancel()
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		if err := database.BoundStatement(ctx, tx, statementLimit); err != nil {
			return err
		}
		if err := purgeExpiredReviews(ctx, tx, e.now()); err != nil {
			return err
		}
		for _, task := range e.tasks {
			for _, step := range task.Steps {
				if err := e.seedStep(ctx, tx, task, step); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// seedStep records a step and nothing more. Its estimate waits for the step's
// first batch: every new step shares this one transaction, so a count read here
// for each of them adds up past its ceiling on a large installation, and then
// no step is recorded and none can start.
func (e *Engine) seedStep(ctx context.Context, tx pgx.Tx, task Task, step Step) error {
	_, err := tx.Exec(ctx, `INSERT INTO data_task_run
 (task_id,step,run_id,task_version,batch_limit,first_release)
 VALUES (@task,@step,@run,@revision,@limit,@release) ON CONFLICT(task_id,step) DO NOTHING`,
		pgx.NamedArgs{paramTask: task.ID, paramStep: step.Name, "run": ids.NewV7(), "revision": task.Version, paramLimit: step.Limit, "release": e.release})
	return err
}
