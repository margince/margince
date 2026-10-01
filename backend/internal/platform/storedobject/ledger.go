// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storedobject

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
)

// Reference declares one kind of stored object: the `<kind>` segment of its
// key (blobstore.WorkspaceKey's `<ws>/<kind>/<id>`), every column a row records
// such a key in, and how long one may stay provisional.
//
// The module that owns the referencing table declares it, because only that
// module knows every column its rows name bytes from; compose collects them.
type Reference struct {
	Kind    string
	Columns []Column
	// Grace is how long a key may stay provisional before the reap treats it
	// as an orphan: past the widest window the writer leaves between the put
	// and the row, since being tight deletes an upload still in flight.
	Grace time.Duration
}

// Column is one table column a referencing row records a key in.
type Column struct {
	Table string
	Name  string
}

// Orphan is one key the reap may delete: provisional past its kind's grace, and
// named by none of its kind's columns.
type Orphan struct {
	StorageKey string
	RecordedAt time.Time
}

// Ledger is the reap's view of the intent ledger, over the declared kinds.
type Ledger struct {
	db   *database.DB
	refs []Reference
}

// errInvalidReference is a declaration the ledger cannot build a safe read from.
var errInvalidReference = errors.New("stored object reference")

// NewLedger answers a ledger over the given declarations, refusing one it could
// not read safely.
func NewLedger(db *database.DB, refs ...Reference) (*Ledger, error) {
	if err := validate(refs); err != nil {
		return nil, err
	}
	return &Ledger{db: db, refs: refs}, nil
}

func validate(refs []Reference) error {
	if len(refs) == 0 {
		return fmt.Errorf("%w: none declared, so the reap could reach nothing", errInvalidReference)
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		switch {
		case ref.Kind == "" || strings.Contains(ref.Kind, "/"):
			return fmt.Errorf("%w: kind %q is not one key segment", errInvalidReference, ref.Kind)
		case seen[ref.Kind]:
			// Two declarations of one kind would each read as complete, and
			// the reap would trust whichever named fewer columns.
			return fmt.Errorf("%w: kind %q is declared twice; declare every column once", errInvalidReference, ref.Kind)
		case ref.Grace <= 0:
			return fmt.Errorf("%w: kind %q has no grace period, which deletes uploads in flight", errInvalidReference, ref.Kind)
		case len(ref.Columns) == 0:
			return fmt.Errorf("%w: kind %q names no referencing column", errInvalidReference, ref.Kind)
		}
		for _, col := range ref.Columns {
			if col.Table == "" || col.Name == "" {
				return fmt.Errorf("%w: kind %q names a column without a table and a name", errInvalidReference, ref.Kind)
			}
		}
		seen[ref.Kind] = true
	}
	return nil
}

// Orphans answers the keys the reap may delete, oldest first.
//
// THE COLUMN JOINS ARE THE SECOND LOCK, not the first. The first is that a key
// is in the ledger at all — nothing but a writer's own declaration puts one
// there. The joins exist because the two are different failures: a clear that
// did not run leaves a live file's key provisional, and a reaper that trusted
// the ledger alone would delete bytes somebody can still open in the product.
//
// `limit` bounds one pass, so a deep backlog does not hold a worker for as long
// as it is deep.
func (l *Ledger) Orphans(ctx context.Context, now time.Time, limit int) ([]Orphan, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	statement, args := l.orphansQuery(now, limit)
	var out []Orphan
	err := l.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, statement, args...)
		if err != nil {
			return fmt.Errorf("list provisional objects: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var o Orphan
			if err := rows.Scan(&o.StorageKey, &o.RecordedAt); err != nil {
				return fmt.Errorf("scan a provisional object: %w", err)
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// orphansQuery spells one arm per declared kind; a key matching no arm is never
// listed, so an undeclared kind is the safe failure — kept, never deleted.
//
// The kind is parsed IN SQL, inside the same predicate the limit bounds. Parsed
// in Go after the limit, keys of an undeclared kind would sit at the head of
// the oldest-first order forever and starve every later pass.
func (l *Ledger) orphansQuery(now time.Time, limit int) (string, []any) {
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	arms := make([]string, 0, len(l.refs))
	for _, ref := range l.refs {
		var arm strings.Builder
		arm.WriteString("(split_part(i.storage_key, '/', 2) = " + arg(ref.Kind) +
			" AND i.recorded_at < " + arg(now.Add(-ref.Grace)))
		for _, col := range ref.Columns {
			arm.WriteString(" AND NOT EXISTS (SELECT 1 FROM " + pgx.Identifier{col.Table}.Sanitize() +
				" r WHERE r." + pgx.Identifier{col.Name}.Sanitize() + " = i.storage_key)")
		}
		arm.WriteString(")")
		arms = append(arms, arm.String())
	}
	return `SELECT i.storage_key, i.recorded_at
		  FROM stored_object_intent i
		 WHERE ` + strings.Join(arms, "\n		    OR ") + `
		 ORDER BY i.recorded_at
		 LIMIT ` + arg(limit), args
}

// Retire removes one key from the ledger once its bytes are gone.
//
// Called AFTER the object-store delete, so a failed delete leaves the key here
// and the next pass tries again. The other order would forget an object that is
// still there, which is the state this whole ledger exists to make impossible.
func (l *Ledger) Retire(ctx context.Context, key string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return l.db.Tx(ctx, func(tx pgx.Tx) error {
		return Clear(ctx, tx, key)
	})
}
