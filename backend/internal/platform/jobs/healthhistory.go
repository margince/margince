// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HistoryFilter binds a traversal to its declared shared kinds and reporting cutoff.
type HistoryFilter struct {
	Dispatchers  []string
	Kind, State  string
	From, Cutoff time.Time
	BeforeID     int64
}

// HealthGroup totals the retained population independently of the bounded run page.
type HealthGroup struct {
	Kind, State       string
	FleetWide         bool
	Count             int
	FirstAt, LatestAt time.Time
}

// History keeps complete group totals separate from the retained run sample.
type History struct {
	Groups []HealthGroup
	Runs   []Failure
	More   bool
}

// ReadHistory reads totals and the page in one read-only repeatable snapshot.
func ReadHistory(ctx context.Context, pool *pgxpool.Pool, filter HistoryFilter) (History, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return History{}, fmt.Errorf("jobs: opening history read: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			slog.ErrorContext(ctx, "closing job history read", "err", err)
		}
	}()
	where, args := historyWhere(filter)
	groups, err := historyGroups(ctx, tx, where, args, filter.Dispatchers)
	if err != nil {
		return History{}, err
	}
	pageWhere := where
	if filter.BeforeID > 0 {
		args = append(args, filter.BeforeID)
		pageWhere += fmt.Sprintf(" AND id < $%d", len(args))
	}
	args = append(args, recentFailureLimit+1)
	q := `SELECT id,kind,state::text,attempt::int,max_attempts::int,
 coalesce(finalized_at,attempted_at,created_at),errors[1]->>'at',errors[cardinality(errors)]->>'error'
 FROM river_job WHERE ` + pageWhere + fmt.Sprintf(` ORDER BY id DESC LIMIT $%d`, len(args))
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return History{}, fmt.Errorf("jobs: reading history page: %w", err)
	}
	runs, err := scanHistory(ctx, rows)
	if err != nil {
		return History{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return History{}, fmt.Errorf("jobs: closing history read: %w", err)
	}
	more := len(runs) > recentFailureLimit
	if more {
		runs = runs[:recentFailureLimit]
	}
	return History{Groups: groups, Runs: runs, More: more}, nil
}

func historyWhere(f HistoryFilter) (string, []any) {
	var args []any
	add := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	where := `state::text IN ('retryable','discarded','cancelled')`
	cutoff, from := add(f.Cutoff), add(f.From)
	where += ` AND created_at <= ` + cutoff + ` AND (state='retryable' OR (coalesce(finalized_at,attempted_at,created_at) >= ` + from + ` AND coalesce(finalized_at,attempted_at,created_at) <= ` + cutoff + `))`
	if f.Kind != "" {
		where += ` AND kind = ` + add(f.Kind)
	}
	if f.State != "" {
		where += ` AND state::text = ` + add(f.State)
	}
	return where, args
}

// historyGroups binds the dispatcher kinds after the shared filter, because the
// page query reads the same filter and Postgres refuses a parameter it never sees.
func historyGroups(ctx context.Context, tx pgx.Tx, where string, args []any, dispatchers []string) ([]HealthGroup, error) {
	groupArgs := append(append([]any{}, args...), dispatchers)
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT kind,state::text,coalesce(kind = ANY($%d::text[]), false),count(*)::int,
 min(coalesce(finalized_at,attempted_at,created_at)),max(coalesce(finalized_at,attempted_at,created_at))
 FROM river_job WHERE `, len(groupArgs))+where+` GROUP BY 1,2,3 ORDER BY 1,2,3`, groupArgs...)
	if err != nil {
		return nil, fmt.Errorf("jobs: grouping history: %w", err)
	}
	defer rows.Close()
	out := []HealthGroup{}
	for rows.Next() {
		var row HealthGroup
		if err := rows.Scan(&row.Kind, &row.State, &row.FleetWide, &row.Count, &row.FirstAt, &row.LatestAt); err != nil {
			return nil, err
		}
		row.FirstAt = row.FirstAt.UTC()
		row.LatestAt = row.LatestAt.UTC()
		out = append(out, row)
	}
	return out, rows.Err()
}

func scanHistory(ctx context.Context, rows pgx.Rows) ([]Failure, error) {
	defer rows.Close()
	out := []Failure{}
	for rows.Next() {
		var f Failure
		var first, reason *string
		if err := rows.Scan(&f.ID, &f.Kind, &f.State, &f.Attempt, &f.MaxAttempts, &f.FailedAt, &first, &reason); err != nil {
			return nil, fmt.Errorf("jobs: scanning history: %w", err)
		}
		f.FailedAt = f.FailedAt.UTC()
		f.FirstFailedAt = parseFirstFailedAt(ctx, f.Kind, first)
		if reason != nil {
			f.StoredReason = *reason
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
