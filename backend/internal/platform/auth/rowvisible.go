// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rowProbe is how strictly a single-row visibility probe judges the row.
type rowProbe int

const (
	// probeScoped skips the query when the caller has no row predicate.
	probeScoped rowProbe = iota
	// probeExisting asks the question for every caller, so an id no row holds
	// is not found even when everything that exists is visible.
	probeExisting
	// probeLive is probeExisting that also refuses an archived row.
	probeLive
)

// EnsureVisible applies the row scope to a single-row operation: get,
// update, archive, advance. Out of scope reads as ErrNotFound, so the
// caller cannot tell "not yours" from "not there".
// Activities scope through their links (the activities module's
// link-walk clause); pipelines have no owner and are governed by object
// grants only.
func EnsureVisible(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	return probeVisible(ctx, tx, table, id, probeScoped)
}

// EnsureVisibleExisting is EnsureVisible that also refuses an id no row holds
// when the caller has no row predicate. A read of a record's sub-resource
// needs it: behind EnsureVisible such an id would be served an empty list.
// EnsureVisible stays as it is for work about a row that was removed.
func EnsureVisibleExisting(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	return probeVisible(ctx, tx, table, id, probeExisting)
}

// EnsureVisibleLive is the strict probe: the row must exist, be live
// (archived_at IS NULL) and pass the caller's row scope.
//
// Art. 17 erasure anonymizes a contact in place, stamps archived_at and leaves
// owner_id alone. A probe without the live filter would still answer "yours"
// for a tombstone that every live read path refuses.
func EnsureVisibleLive(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	return probeVisible(ctx, tx, table, id, probeLive)
}

func probeVisible(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, mode rowProbe) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)

	terms, err := rowVisibleTerms(ctx, table, arg, mode == probeLive)
	if err != nil {
		return err
	}
	if len(terms) == 0 && mode == probeScoped {
		return nil
	}
	where := strings.Join(append([]string{fmt.Sprintf("id = $%d", idPos)}, terms...), " AND ")

	var visible bool
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE %s)`, table, where),
		args...).Scan(&visible)
	if err != nil {
		return err
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}

// rowVisibleTerms is the row half of a visibility probe, shared by the
// single-row probe above and PassingSubset: the live filter, then the scope.
func rowVisibleTerms(ctx context.Context, table string, arg func(any) int, live bool) ([]string, error) {
	clause, err := ScopeClauseFor(ctx, table, "", arg)
	if err != nil {
		return nil, err
	}
	var terms []string
	if live {
		terms = append(terms, "archived_at IS NULL")
	}
	if clause != "" {
		terms = append(terms, clause)
	}
	return terms, nil
}
