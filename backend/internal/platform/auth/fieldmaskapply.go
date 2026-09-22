// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The per-row pass that turns a caller's masks into a rendered page: collect
// what this reader may not have, withhold it, name it. One spelling, because
// four record types owe the same answer and the copy that forgets the
// write-authority arm is the one that ships the figure.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ApplyFieldMasks withholds, per row, the columns this caller's role does not
// read, and names them on the row so a reader can tell withheld from empty.
//
// Generic over the row type with caller-supplied accessors, like StampWritable
// beside it: platform owns no contract type and must not learn one. withhold is
// the module's registry of what each field's absence means on its own record;
// extra contributes names the module collected for itself — the references a
// deal may not hand back are a question about ROWS that ends the same way.
func ApplyFieldMasks[T any](ctx context.Context, tx pgx.Tx, object string,
	rows []T,
	id func(T) ids.UUID,
	withhold map[string]func(*T),
	setMasked func(*T, []string),
	extra func(rowIndex int) []string,
) error {
	if len(rows) == 0 {
		return nil
	}
	names, err := maskedNamesPerRow(ctx, tx, object, rows, id, extra)
	if err != nil {
		return err
	}
	for i := range rows {
		withholdNamed(&rows[i], names[i], withhold, setMasked)
	}
	return nil
}

// withheldNames is the ordered set of fields withheld from ONE row. Ordered
// because masked_fields goes on the wire, and a client diffing two reads of one
// row must not see the list reshuffle under it.
type withheldNames []string

func (w *withheldNames) add(fields ...string) {
	for _, field := range fields {
		if !slices.Contains(*w, field) {
			*w = append(*w, field)
		}
	}
}

// maskedNamesPerRow collects what each row of the page withholds, before
// anything is nulled: the role's masks, answered against this row's write
// authority, and whatever the module's own pass adds.
func maskedNamesPerRow[T any](ctx context.Context, tx pgx.Tx, object string,
	rows []T, id func(T) ids.UUID, extra func(rowIndex int) []string,
) ([]withheldNames, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return nil, err
	}
	outsideAuthority, insideAuthority := MaskedFields(p, object, false), MaskedFields(p, object, true)
	// The page buys the write-authority statement only where the answer could
	// change what it withholds, so an unconditioned page pays nothing. A
	// condition the object cannot answer lifts nothing in MaskedFields either,
	// which is what keeps this off WritableSubset's non-shareable refusal.
	var writable map[ids.UUID]bool
	if !slices.Equal(outsideAuthority, insideAuthority) {
		if writable, err = writeAuthorityOfPage(ctx, tx, object, rows, id); err != nil {
			return nil, err
		}
	}
	names := make([]withheldNames, len(rows))
	for i := range rows {
		if writable[id(rows[i])] {
			names[i].add(insideAuthority...)
		} else {
			names[i].add(outsideAuthority...)
		}
		if extra != nil {
			names[i].add(extra(i)...)
		}
	}
	return names, nil
}

// writeAuthorityOfPage answers in ONE statement which rows of the page the
// caller could change. Archived rows stay in the answer: a mask applies to a
// record a caller may still READ, and StampWritable's edit affordance is the
// flag that owes the liveness question.
func writeAuthorityOfPage[T any](ctx context.Context, tx pgx.Tx, object string,
	rows []T, id func(T) ids.UUID,
) (map[ids.UUID]bool, error) {
	rowIDs := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		rowIDs = append(rowIDs, id(row))
	}
	return WritableSubset(ctx, tx, object, rowIDs)
}

// withholdNamed nulls every named field on the row and records the names it
// actually nulled. A name with no withhold func is DROPPED, not reported:
// naming a field in masked_fields while still sending its value is a worse
// answer than either half alone.
func withholdNamed[T any](row *T, names withheldNames,
	withhold map[string]func(*T), setMasked func(*T, []string),
) {
	named := make([]string, 0, len(names))
	for _, field := range names {
		hide, known := withhold[field]
		if !known {
			continue
		}
		hide(row)
		named = append(named, field)
	}
	if len(named) > 0 {
		setMasked(row, named)
	}
}
