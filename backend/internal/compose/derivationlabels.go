// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The drill-through's NAMES: each source row carries the display name of the
// record it stands for, so "explain this number" answers with the deals a
// reader recognises rather than the UUIDs the query happened to select.
//
// The name comes from the SAME seam the attention feed uses
// (attentionnames.go): one gated read per type, under the READER's grants,
// absent when the caller may not read it. Nothing here holds authority of
// its own — a row whose name is withheld keeps its id and loses its label,
// which is the contract's answer everywhere else a label travels.

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// derivationLabelColumn is where a row's display name lands. It is not a
// dimension the caller can group or filter by: the plan builds its columns
// from the spec's vocabulary, and this one is appended after the query has
// run, so it can never reach SQL.
const derivationLabelColumn = "label"

// labelDerivationRows names each source row through the caller's own grants.
//
// A row is named when the seam answers for its id and stays unnamed
// otherwise, which covers three cases with one behaviour: the caller may not
// read that type at all, this particular row is outside their row scope, or
// the entity has no label lane. In every one of them the id still travels
// and the arithmetic is untouched — the drill-through still reconciles to
// the number it explains, because a label is presentation and never a term
// in the sum.
//
// No names seam wired means no labels, not an error: the report engine
// serves the agent surface too, where a tool answers in ids.
//
// Reports whether any row was named, so the caller can announce the label
// column only when it is really there. A column advertised on rows that
// never carry it reads to a renderer as a column of blanks.
func labelDerivationRows(ctx context.Context, names attention.Names, entity string, rows []map[string]any) bool {
	if names == nil || len(rows) == 0 {
		return false
	}
	want := make([]ids.UUID, 0, len(rows))
	seen := make(map[ids.UUID]struct{}, len(rows))
	for _, row := range rows {
		id, ok := rowRecordID(row)
		if !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		want = append(want, id)
	}
	if len(want) == 0 {
		return false
	}
	labels, err := names.Labels(ctx, entity, want)
	if err != nil {
		// A label that will not resolve costs the label. The rows and their
		// aggregates are already correct and already row-scoped; failing the
		// whole explanation because its display names did not load would
		// hide a number the caller is entitled to see.
		return false
	}
	var named bool
	for _, row := range rows {
		id, ok := rowRecordID(row)
		if !ok {
			continue
		}
		if label, has := labels[id]; has {
			row[derivationLabelColumn] = label
			named = true
		}
	}
	return named
}

// rowRecordID reads the row identity the plan always selects first. It is a
// wire value by the time it lands here, so a string is what the seam gets
// back from a UUID column.
func rowRecordID(row map[string]any) (ids.UUID, bool) {
	raw, ok := row["id"].(string)
	if !ok {
		return ids.UUID{}, false
	}
	id, err := ids.Parse(raw)
	if err != nil {
		return ids.UUID{}, false
	}
	return id, true
}
