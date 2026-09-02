// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// What a proposal's target is CALLED, recorded when the proposal is staged.
//
// Roughly half the stageable kinds carry no typed payload — the raw arguments a
// tool was called with, an automation action's args, a canonicalized HTTP body —
// so a card has nothing but the summary to work from, and the summaries those
// paths compose name the record by uuid. This is the one field that lets the
// card say which record without asking the reader to open the payload.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// targetLabelColumns names the column that holds each target type's display
// name, for the types that have one.
//
// A SUBSET of versionTables on purpose, and not derived from it: being
// version-pinnable says the row bumps a counter on write, which is a different
// fact from having something a person calls it by. A relationship is an edge, a
// saved view is a per-user filter — a caption drawn from either would be
// labelling the proposal with a word its reader never uses.
//
// Absent from this map means "no name to record", which the column stores as
// NULL and the card reads as "say nothing" rather than "say unknown".
//
// The column is a person's own name where the proposal is about a person, so the
// Art. 17 redaction clears it with the summary and the payload —
// privacy.blankStagedProposal, held by gates/piicolumncoverage_test.go, which
// refuses a text column on a PII table that the redaction neither clears nor
// accounts for.
// columnName is the column several of these types happen to share, lifted out
// so the map below reads as a set of choices rather than a repeated literal.
// Sharing a column name is a coincidence of schema, not a rule: a type is in
// this map because somebody calls the row by that field, and the next one may
// spell it differently.
const columnName = "name"

var targetLabelColumns = map[string]string{
	tablePerson:       "full_name",
	tableOrganization: "display_name",
	tableDeal:         columnName,
	tableLead:         "full_name",
	tableProject:      columnName,
	tableList:         columnName,
	targetProduct:     columnName,
	targetTag:         columnName,
}

// targetLabel reads what the proposal's target is called, at staging time.
//
// AT STAGING, not at read: the target may be renamed, archived or merged before
// anybody opens the inbox, and a card that resolved the name then would put a
// word in front of the approver that the proposal was never about. The version
// pin beside it is taken in the same transaction for the same reason.
//
// A target that has vanished between the caller reading it and this insert is
// not an error here: the staging is refused elsewhere if the row matters, and a
// missing caption is not a reason to fail a proposal that is otherwise sound.
func targetLabel(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) *string {
	column, named := targetLabelColumns[table]
	if !named || id.IsZero() {
		return nil
	}
	var label *string
	if err := tx.QueryRow(ctx,
		`SELECT `+column+` FROM `+table+` WHERE id = $1`, id).Scan(&label); err != nil {
		return nil
	}
	if label != nil && strings.TrimSpace(*label) == "" {
		// An empty name is not a name. Storing it would put a blank caption
		// where the card would otherwise say nothing, which reads as a record
		// whose name somebody deleted.
		return nil
	}
	return label
}
