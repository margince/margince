// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The activity projection, for a reader that composes its own FROM and WHERE.
//
// The contact 360's timeline is one: the row-scope predicates it applies —
// which activities a contact reaches, what a project narrowing removes, what
// the discover gate admits — are its own question and belong to it. The SELECT
// list is not. It hand-wrote a copy of the projection, and the copy silently
// failed to grow three times: `channel_provider`, so a message rendered as the
// bare word "message"; `version`, so the audience control could not write; and
// `source_system`, so a meeting read from a transcript reached the timeline
// with nothing to say it was one. Each was caught by a test naming that one
// column, and none of them could have caught the next.
//
// So the list is handed out instead of copied. The EXTRAS a caller adds come
// after it, which is what lets one Scan take both: the shared destinations in
// the projection's order, then the caller's own.

import (
	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// ActivityProjection is the SELECT list every activity read scans, rendered
// with this query's own audience test.
//
// contentArm is auth.ActivityAudienceArm rendered for the caller's arguments.
// It lands in the final column, content_available, which is what decides
// whether a row's content reaches the caller or only its markers — so a reader
// that composed a different test there would be answering a different question
// about the same rows.
func ActivityProjection(contentArm string) string { return activityColumns(contentArm) }

// ActivityRow is one row of that projection, mid-flight.
//
// It exists so a caller can scan the shared columns without seeing what they
// scan INTO: the destinations are part of each column's declaration, and a
// caller holding them could reorder the two halves the projection exists to
// keep together.
type ActivityRow struct{ scan activityScan }

// Targets are the shared destinations, in the projection's order. A caller
// with extra columns appends its own:
//
//	row := &activities.ActivityRow{}
//	err := rows.Scan(append(row.Targets(), &filedHere)...)
func (r *ActivityRow) Targets() []any { return activityScanTargets(&r.scan) }

// Record finishes the row: the conversions the scan could not do, and the
// content the audience test decided to withhold.
func (r *ActivityRow) Record() crmcontracts.Activity { return r.scan.record() }

// ScanActivityRow reads one row of the projection plus the caller's own
// trailing columns, and answers the finished record.
//
// The extras come last because that is the only position a shared list can
// promise: anywhere else and the caller would be choosing an index into a list
// it does not own.
func ScanActivityRow(rows pgx.Rows, extras ...any) (crmcontracts.Activity, error) {
	row := &ActivityRow{}
	if err := rows.Scan(append(row.Targets(), extras...)...); err != nil {
		return crmcontracts.Activity{}, err
	}
	return row.Record(), nil
}
