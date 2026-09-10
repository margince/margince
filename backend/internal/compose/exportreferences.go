// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an exported row may say about the records it POINTS AT.
//
// A row the caller may read is not a licence to read everything it names. An
// organization can be capture-private to the colleague who captured it and a
// project keeps its own/team scope, so a deal — which every seat of the
// workspace reads, because a deal is customer identity — carries references
// its reader's own organization and project reads would refuse.
//
// Field masks do not answer this. maskedRowSelects applies the caller's ROLE
// masks, which ask what this role may see of any row — never what THIS reader
// may see of the row this one names. Without the second question an export
// hands back the id of a company whose own row the same bundle withholds, and
// an id is an existence oracle: it says the record is there and that someone
// in this workspace has it.
//
// DERIVED, not listed. The pairs come from the schema's own foreign keys to
// row-scoped tables, so a column added next year is covered the day it is
// declared rather than the day somebody remembers this file;
// TestEveryExportedReferenceToARowScopedTableIsWithheld holds the derivation
// against the head catalog and fails on a pair nobody added.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// exportedReference is one column that names a row-scoped record.
type exportedReference struct {
	column string
	// target is the table the column references — what a visibility probe is
	// asked about, and why two columns of one table can need different answers
	// (a deal's partner and its customer are both organizations; its project
	// is not).
	target string
}

// The reference columns, prefixed for what they are.
//
// NOT report.go's colPartnerOrgID and friends, which carry the same words with
// a `t.` alias in front: those name a column inside one report's own SELECT,
// and these name a column of a row the export hands back. Two vocabularies
// that happen to share their spelling.
const (
	refColMergedInto        = "merged_into_id"
	refColOrganization      = "organization_id"
	refColPartnerOrg        = "partner_org_id"
	refColProject           = "project_id"
	refColPromotedPerson    = "promoted_person_id"
	refColQualifiedDeal     = "qualified_deal_id"
	refColConvertedFromLead = "converted_from_lead_id"
	refColParentOrg         = "parent_org_id"
)

// referencesByTable is every column an exported row carries that names a
// record the reader may not be able to open.
//
// Only references BETWEEN row-scoped tables are here. A column naming a table
// every reader may read discloses nothing by naming it, and withholding one
// would blank a field for no reason — the availability regression that wears a
// security fix's clothes.
//
// Held by: TestEveryExportedReferenceToARowScopedTableIsWithheld (backend/gates/exportedreferences_test.go)
//
// Keyed by replayscope.go's table constants: these are DATABASE tables, the
// same vocabulary a row-scope clause is built on, and not the agent record
// types or report dimensions that happen to share their spelling.
var referencesByTable = map[string][]exportedReference{
	tableDeal: {
		{column: refColOrganization, target: tableOrganization},
		{column: refColPartnerOrg, target: tableOrganization},
		{column: refColProject, target: tableProject},
	},
	tableLead: {
		{column: refColMergedInto, target: tableLead},
		{column: refColProject, target: tableProject},
		{column: refColPromotedPerson, target: tablePerson},
		{column: refColQualifiedDeal, target: tableDeal},
	},
	tableOrganization: {
		{column: refColMergedInto, target: tableOrganization},
		{column: refColParentOrg, target: tableOrganization},
	},
	tablePerson: {
		{column: refColConvertedFromLead, target: tableLead},
		{column: refColMergedInto, target: tablePerson},
	},
	tableProject: {
		{column: refColOrganization, target: tableOrganization},
	},
}

// withholdUnreadableReferences blanks, in place, every reference an exported
// row carries to a record this reader could not open.
//
// Positional because the export carries rows as value slices beside their
// column names; the index is resolved once per reference rather than per row.
//
// ONE probe per referenced table for the whole page, never one per row: an
// export is the shape most likely to turn a per-row question into thousands of
// round trips, and VisibleSubset answers a whole id set in one statement.
func withholdUnreadableReferences(
	ctx context.Context, tx pgx.Tx, table string, columns []string, rows [][]any,
) error {
	refs := referencesByTable[table]
	if len(refs) == 0 || len(rows) == 0 {
		return nil
	}
	for _, ref := range refs {
		at := indexOfColumn(columns, ref.column)
		if at < 0 {
			// The export does not carry this column, so there is nothing to
			// withhold. Not an error: exportableColumns is the live catalog's
			// answer and a member may legitimately export a subset.
			continue
		}
		referenced := referencedIDs(rows, at)
		// VisibleSubset answers an empty set without a round trip, so a page
		// naming nothing pays for nothing.
		visible, err := auth.VisibleSubset(ctx, tx, ref.target, referenced)
		if err != nil {
			return fmt.Errorf("compose: reading which %s rows the exporter may open: %w", ref.target, err)
		}
		for _, row := range rows {
			id, named := referencedID(row, at)
			if named && !visible[id] {
				row[at] = nil
			}
		}
	}
	return nil
}

func indexOfColumn(columns []string, want string) int {
	for i, c := range columns {
		if c == want {
			return i
		}
	}
	return -1
}

// referencedIDs collects the ids one column of the page names, duplicates and
// all — VisibleSubset keys its answer by id, so a repeated reference costs
// nothing extra and de-duplicating here would be a second pass for no gain.
func referencedIDs(rows [][]any, at int) []ids.UUID {
	out := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		if id, named := referencedID(row, at); named {
			out = append(out, id)
		}
	}
	return out
}

// referencedID reads one row's reference, and says whether there is one.
//
// A row may carry the column as nil (nothing linked) or as a value a mask
// already blanked, and both are "names nothing" rather than an error: pgx
// hands a uuid column back as [16]byte, and anything else here is a column
// that is not the reference this pair declared.
func referencedID(row []any, at int) (ids.UUID, bool) {
	if at >= len(row) || row[at] == nil {
		return ids.UUID{}, false
	}
	switch v := row[at].(type) {
	case [16]byte:
		return ids.UUID(v), true
	case ids.UUID:
		return v, true
	default:
		return ids.UUID{}, false
	}
}
