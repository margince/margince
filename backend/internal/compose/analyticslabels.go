// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Display names for the record ids an analytics answer groups by.
//
// A grouping by company_id answers in uuids. The builder needs the companies'
// names, and asking for them one GET per id is a hundred requests for one
// answer. So the answer carries them, read through the same Names seam the
// drill-through's row labels use: one read per record type, under the reader's
// own grants, after the answer's transaction has closed.

import (
	"context"
	"log/slog"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// analyticsIDLabels names every id a question's group columns carry, keyed by
// column and then id. An id the reader may not name is absent, and so is a
// column with nothing named. A withheld row carries null keys, so it
// contributes no id to name.
func analyticsIDLabels(
	ctx context.Context, names attention.Names, q analyticsquery.Query, rows []map[string]any,
) map[string]map[string]string {
	spec, ok := prebuiltReports[q.Entity]
	if names == nil || !ok {
		return nil
	}
	wanted := map[string]map[ids.UUID]struct{}{}
	columnType := map[string]string{}
	for _, column := range q.GroupBy {
		recordType, named := groupRecordType(spec, column)
		if !named {
			continue
		}
		columnType[column] = recordType
		for _, row := range rows {
			if id, isID := columnRecordID(row, column); isID {
				if wanted[recordType] == nil {
					wanted[recordType] = map[ids.UUID]struct{}{}
				}
				wanted[recordType][id] = struct{}{}
			}
		}
	}

	byType := make(map[string]map[ids.UUID]string, len(wanted))
	for recordType, set := range wanted {
		labels, err := names.Labels(ctx, recordType, slices.Collect(maps.Keys(set)))
		if err != nil {
			// The names cost their column and nothing else: the answer is
			// already correct and already scoped, and a display name that did
			// not load must not hide a number the reader is entitled to.
			slog.WarnContext(ctx, "analytics: naming the ids of a grouped column",
				"record_type", recordType, "err", err)
			continue
		}
		byType[recordType] = labels
	}

	out := map[string]map[string]string{}
	for column, recordType := range columnType {
		for _, row := range rows {
			id, isID := columnRecordID(row, column)
			label, named := byType[recordType][id]
			if !isID || !named {
				continue
			}
			if out[column] == nil {
				out[column] = map[string]string{}
			}
			out[column][id.String()] = label
		}
	}
	return out
}

// groupRecordType is the record type a dimension's value is the id of, read
// from the declarations the spec already makes for gating that reference: its
// row scope, its filter's read gate, or — on a project report — the row's own
// id.
func groupRecordType(spec reportSpec, dimension string) (string, bool) {
	expr, ok := spec.dimensions[dimension]
	if !ok {
		return "", false
	}
	if table, scoped := spec.referenceScopes[expr]; scoped {
		return table, true
	}
	if table, gated := spec.filterScopes[dimension]; gated {
		return table, true
	}
	if expr == colProjectRowID {
		return string(spec.entity), true
	}
	return "", false
}

// columnRecordID reads a uuid out of one of a row's columns, which a UUID
// column already is by the time a row is shaped.
func columnRecordID(row map[string]any, column string) (ids.UUID, bool) {
	raw, ok := row[column].(string)
	if !ok {
		return ids.UUID{}, false
	}
	id, err := ids.Parse(raw)
	return id, err == nil
}
