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

// Item is one record on one holder's list. Subject is an activity id for the
// kinds this proof carries.
type Item struct {
	Kind    string
	Subject ids.UUID
	Holder  ids.UUID
	SortAt  *time.Time
	Version int64
}

// Page is the holder's readable items in list order, and how many there are.
type Page struct {
	Items []Item
	Total int
}

// Store reads and writes work_item under the workspace transaction.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore binds the store to a pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Replace sets the open items of one kind for one holder to exactly items,
// as a producer that recomputed that holder computes them. A generation older
// than the stored one changes nothing, so a late producer run cannot bring
// back an item a newer run resolved.
func (s *Store) Replace(ctx context.Context, holder ids.UUID, kind string, items []Item, generation int64) error {
	subjects := make([]ids.UUID, 0, len(items))
	sortAt := make([]*time.Time, 0, len(items))
	for _, it := range items {
		subjects = append(subjects, it.Subject)
		sortAt = append(sortAt, it.SortAt)
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO work_item (kind, subject_id, holder_id, sort_at, version, resolved_at)
			SELECT $1, u.subject, $2, u.sort_at, $3, NULL
			  FROM unnest($4::uuid[], $5::timestamptz[]) AS u(subject, sort_at)
			ON CONFLICT (kind, subject_id, holder_id) DO UPDATE
			   SET sort_at = excluded.sort_at, version = excluded.version,
			       resolved_at = NULL, updated_at = now()
			 WHERE work_item.version <= excluded.version`,
			kind, holder, generation, subjects, sortAt); err != nil {
			return fmt.Errorf("worklist: writing %s items: %w", kind, err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE work_item SET resolved_at = now(), version = $3, updated_at = now()
			 WHERE holder_id = $1 AND kind = $2 AND resolved_at IS NULL
			   AND version < $3 AND subject_id <> ALL($4::uuid[])`,
			holder, kind, generation, subjects); err != nil {
			return fmt.Errorf("worklist: resolving %s items: %w", kind, err)
		}
		return nil
	})
}

// Read answers the holder's open items of kinds whose record the CALLER may
// read now. The gate runs over the whole set, not the page, so Total never
// counts a record the caller cannot open.
func (s *Store) Read(ctx context.Context, holder ids.UUID, kinds []string, limit int) (Page, error) {
	args := []any{holder, kinds}
	arg := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	readable, err := auth.ContentReadableActivitiesSQL(ctx, "SELECT subject_id FROM mine", arg)
	if err != nil {
		return Page{}, err
	}
	statement := `
		WITH mine AS MATERIALIZED (
		  SELECT kind, subject_id, sort_at FROM work_item
		   WHERE holder_id = $1 AND kind = ANY($2) AND resolved_at IS NULL),
		gated AS (
		  SELECT m.* FROM mine m WHERE m.subject_id IN (` + readable + `))
		SELECT kind, subject_id, sort_at, count(*) OVER () FROM gated
		 ORDER BY sort_at NULLS LAST, subject_id
		 LIMIT $` + fmt.Sprint(arg(limit))
	var page Page
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, statement, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			it := Item{Holder: holder}
			if err := rows.Scan(&it.Kind, &it.Subject, &it.SortAt, &page.Total); err != nil {
				return err
			}
			page.Items = append(page.Items, it)
		}
		return rows.Err()
	})
	return page, err
}
