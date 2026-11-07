// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package worklist

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Item is one record on one holder's list.
type Item struct {
	Kind    string
	Subject ids.UUID
	Holder  ids.UUID
	SortAt  *time.Time
}

// Store reads and writes work_item under the workspace transaction.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore binds the store to a pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Rebuild sets one holder's open items of one kind to exactly items. The
// caller runs it under the holder's lock, and the generation is drawn after
// the lock is held, so a later generation is always a newer reading. Resolved
// rows stay as tombstones carrying their generation, so a run that drew an
// older one can neither reopen nor re-add what a newer run settled.
func (s *Store) Rebuild(ctx context.Context, holder ids.UUID, kind string, items []Item) (int64, error) {
	subjects := make([]ids.UUID, 0, len(items))
	sortAt := make([]*time.Time, 0, len(items))
	for _, it := range items {
		subjects = append(subjects, it.Subject)
		sortAt = append(sortAt, it.SortAt)
	}
	var generation int64
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text || $2, 0))`, holder, kind); err != nil {
			return fmt.Errorf("worklist: locking %s: %w", kind, err)
		}
		if err := tx.QueryRow(ctx, `SELECT nextval('work_item_gen')`).Scan(&generation); err != nil {
			return fmt.Errorf("worklist: drawing a generation: %w", err)
		}
		return s.write(ctx, tx, holder, kind, subjects, sortAt, generation)
	})
	return generation, err
}

// RebuildAt is Rebuild with a generation the caller already drew, which is how
// a test plays a run that lost a race.
func (s *Store) RebuildAt(ctx context.Context, holder ids.UUID, kind string, items []Item, generation int64) error {
	subjects := make([]ids.UUID, 0, len(items))
	sortAt := make([]*time.Time, 0, len(items))
	for _, it := range items {
		subjects = append(subjects, it.Subject)
		sortAt = append(sortAt, it.SortAt)
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return s.write(ctx, tx, holder, kind, subjects, sortAt, generation)
	})
}

func (s *Store) write(
	ctx context.Context, tx pgx.Tx, holder ids.UUID, kind string, subjects []ids.UUID, sortAt []*time.Time, generation int64,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO work_item (kind, subject_id, holder_id, sort_at, gen, open)
		SELECT $1, u.subject, $2, u.sort_at, $3, true
		  FROM unnest($4::uuid[], $5::timestamptz[]) AS u(subject, sort_at)
		ON CONFLICT (kind, subject_id, holder_id) DO UPDATE
		   SET sort_at = excluded.sort_at, gen = excluded.gen, open = true, updated_at = now()
		 WHERE work_item.gen < excluded.gen`,
		kind, holder, generation, subjects, sortAt); err != nil {
		return fmt.Errorf("worklist: writing %s items: %w", kind, err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE work_item SET open = false, gen = $3, updated_at = now()
		 WHERE holder_id = $1 AND kind = $2 AND open AND gen < $3 AND subject_id <> ALL($4::uuid[])`,
		holder, kind, generation, subjects); err != nil {
		return fmt.Errorf("worklist: resolving %s items: %w", kind, err)
	}
	return nil
}

// HoldersOf answers who holds any of these records now, open or not, so a
// router can rebuild a holder a record is leaving: the event that moved it does
// not say where it came from.
func (s *Store) HoldersOf(ctx context.Context, kind string, subjects []ids.UUID) ([]ids.UUID, error) {
	var holders []ids.UUID
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT coalesce(array_agg(DISTINCT holder_id), '{}') FROM work_item
			 WHERE kind = $1 AND subject_id = ANY($2) AND open`, kind, subjects).Scan(&holders)
	})
	return holders, err
}

// Candidates answers the holder's open items of one kind, unchecked. A caller
// hands them to the owning module, which applies its own rule to them.
func (s *Store) Candidates(ctx context.Context, holder ids.UUID, kind string) ([]ids.UUID, error) {
	var subjects []ids.UUID
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT coalesce(array_agg(subject_id ORDER BY sort_at NULLS LAST, subject_id), '{}')
			  FROM work_item WHERE holder_id = $1 AND kind = $2 AND open`, holder, kind).Scan(&subjects)
	})
	return subjects, err
}

// Gated answers the holder's open activity items of one kind that the CALLER
// may discover now, in list order, and how many there are up to countCap. The
// gate runs over the whole set before the count, so a total never includes a
// record the caller cannot see; past the cap the count is a floor.
func (s *Store) Gated(ctx context.Context, holder ids.UUID, kind string, countCap int) ([]Item, int, bool, error) {
	args := []any{holder, kind}
	arg := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	gate, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, 0, false, err
	}
	statement := `
		SELECT w.subject_id, w.sort_at FROM work_item w JOIN activity a ON a.id = w.subject_id
		 WHERE w.holder_id = $1 AND w.kind = $2 AND w.open AND ` + gate + `
		 ORDER BY w.sort_at NULLS LAST, w.subject_id
		 LIMIT $` + fmt.Sprint(arg(countCap+1))
	var items []Item
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, statement, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			it := Item{Kind: kind, Holder: holder}
			if err := rows.Scan(&it.Subject, &it.SortAt); err != nil {
				return err
			}
			items = append(items, it)
		}
		return rows.Err()
	})
	more := len(items) > countCap
	if more {
		items = items[:countCap]
	}
	return items, len(items), more, err
}
