// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the dry run TELLS a person before they approve it.
//
// The engine's report counts rows it would touch; this walks the file again and
// asks the writers what each row would actually do — created, updated, unchanged,
// refused, or already here. That second pass is the value of a preview: an
// approval is a decision about what the report said.
//
// The four counts sum to rows_read and no row is counted twice among them.
// `duplicates` sits outside that sum deliberately: it is a disclosure about rows
// already counted elsewhere, not a fifth outcome.

import (
	"context"

	"github.com/gradionhq/margince/backend/internal/modules/migration"
)

func refinePrediction(ctx context.Context, source *migration.CSVSource, writers *csvWriters, object string, report migration.Report) (migration.Report, error) {
	p, err := predictPages(ctx, source, writers, object)
	if err != nil {
		return migration.Report{}, err
	}
	for i := range report.Objects {
		if report.Objects[i].Object != object {
			continue
		}
		report.Objects[i].WillCreate = p.created
		report.Objects[i].WillUpdate = p.updated
		report.Objects[i].Unchanged = p.unchanged
		report.Objects[i].Skipped = append(report.Objects[i].Skipped, p.skipped...)
		report.Objects[i].Collisions = append(report.Objects[i].Collisions, p.collisions...)
		report.Objects[i].Duplicates += p.duplicates
	}
	return report, nil
}

// predictPages walks the source and tallies what the commit would do with each
// row, page by page, the same way the engine will walk it.
func predictPages(ctx context.Context, source *migration.CSVSource, writers *csvWriters, object string) (p prediction, err error) {
	collision, skipReason := collisionWordingFor(object)
	for offset := 0; ; offset += importPredictPage {
		rows, err := source.Rows(ctx, object, offset, importPredictPage)
		if err != nil {
			return prediction{}, err
		}
		for _, row := range rows {
			outcome, refusal, err := writers.predictRow(ctx, row)
			if err != nil {
				return prediction{}, err
			}
			switch outcome {
			case predictCreate:
				p.created++
			case predictUpdate:
				p.updated++
			case predictUnchanged:
				p.unchanged++
			case predictUnwritable:
				// Disclosed as a skip rather than counted as a create: the
				// commit will refuse this row, and the report exists to say so
				// before a human approves it.
				p.skipped = append(p.skipped, migration.SkippedRow{
					ExternalID: row.ExternalID,
					Reason:     refusal,
				})
			case predictCollidesSkipped:
				// The run asked to skip duplicates, so this row is a skip and
				// the preview says so — the commit must not be the first place
				// a person learns the row did not land.
				p.duplicates++
				p.skipped = append(p.skipped, migration.SkippedRow{
					ExternalID: row.ExternalID,
					Reason:     skipReason,
				})
			case predictCollides:
				p.duplicates++
				// Still a create — the commit lands it and files a review pair
				// — so it is counted as one. The warning rides separately:
				// Skipped is load-bearing arithmetic (the contract's four
				// counts sum to rows_read, and `unchanged` is derived by
				// subtracting them), so a row counted as BOTH created and
				// skipped would report two outcomes for one row.
				p.created++
				p.collisions = append(p.collisions, migration.SkippedRow{
					ExternalID: row.ExternalID,
					Reason:     collision,
				})
			}
		}
		if len(rows) < importPredictPage {
			return p, nil
		}
	}
}

// collisionDisclosure is what the report says about a row naming a company the
// CRM already holds. It is not a refusal: the commit creates the row and files
// the pair on the dedupe review queue, exactly as a manual create would. The
// disclosure exists so a person reading "create 3" before approving is not
// surprised by a duplicate afterwards.
const collisionDisclosure = "a company of this name is already in the CRM; " +
	"importing this row creates a second one and files the pair for review"

// duplicateSkipReason is what the report says about a row the run chose to
// leave alone, under `on_duplicate: skip`.
const duplicateSkipReason = "a company of this name is already in the CRM, and this run was asked to skip duplicates"

// The same two sentences for a person run, which must not tell its reader that a
// company was found.
//
// The disclosure is not merely the company one with a noun swapped. An address
// the estate already holds is REFUSED by the store — uq_person_email_dedupe is a
// real key, where a company name is not — so promising "a second one is created"
// would be false for the very case a reader is most likely to hit.
const personCollisionDisclosure = "someone matching this row is already in the CRM; " +
	"a row naming an address already held is refused, and a near match creates and files the pair for review"

const personDuplicateSkipReason = "someone matching this row is already in the CRM, and this run was asked to skip duplicates"

// collisionWordingFor names the sentences that match what this run imports.
func collisionWordingFor(object string) (disclosure, skipReason string) {
	if object == migration.ObjectPerson {
		return personCollisionDisclosure, personDuplicateSkipReason
	}
	return collisionDisclosure, duplicateSkipReason
}

// prediction is what one walk of the source concluded: the three outcomes a
// commit can have, plus the rows it will refuse outright.
type prediction struct {
	created   int
	updated   int
	unchanged int
	skipped   []migration.SkippedRow
	// collisions are rows that WILL land and also name a company the CRM
	// already holds. Kept apart from skipped because each is counted in
	// created — see the disposition arithmetic above.
	collisions []migration.SkippedRow
	// duplicates counts the same rows. It is reported to the human as its own
	// number ("100 companies, 94 duplicates") and never summed with the four.
	duplicates int
}
