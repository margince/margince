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

	"github.com/margince/margince/backend/internal/modules/migration"
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
		report.Objects[i].WillDuplicate += p.duplicates
	}
	return withPredictedLinks(ctx, source, writers, report, p.absent)
}

// withPredictedLinks answers, before anything is written, how many of the
// employer links the file asks for will actually find a company.
//
// The engine's own dry run counts edges OFFERED — it has no writer, so it cannot
// resolve an endpoint. That number alone would tell a person approving a file of
// 12,000 contacts that 12,000 links are coming, when the truthful answer might be
// 3,000 and 9,000 companies nobody has imported yet. So the resolvable half is
// computed here through the SAME resolver the commit will use, and the names that
// resolve to nothing are listed rather than counted — the answer a person needs
// is WHICH company was not found, because that is the row they have to go fix.
//
// Only the COMPANY end is resolved. At dry-run time no person has landed, so the
// person end would answer "not imported" for every row, and predicting that would
// be worse than saying nothing. The asymmetry is deliberate: a preview reports
// what it can honestly know, and claiming more is the preview/commit disagreement
// this whole pass exists to prevent.
func withPredictedLinks(ctx context.Context, source *migration.CSVSource, writers *csvWriters,
	report migration.Report, absent []string,
) (migration.Report, error) {
	edges, err := source.Associations(ctx)
	if err != nil {
		return migration.Report{}, err
	}
	if len(edges) == 0 {
		return report, nil
	}
	// A row the commit will refuse lands no person, so its employer link cannot
	// be written either. Counting it as resolvable would promise a link whose
	// person is never there — preview says "linked", commit says "nobody to link
	// to", which is the disagreement this whole pass exists to prevent.
	willNotLand := make(map[string]bool, len(absent))
	for _, id := range absent {
		willNotLand[id] = true
	}
	resolvable := 0
	for _, edge := range edges {
		if willNotLand[edge.FromID] {
			report.AssociationsSkipped = append(report.AssociationsSkipped, migration.SkippedAssoc{
				From:   edge.FromType + "/" + edge.FromID,
				To:     edge.ToType + "/" + edge.ToID,
				Reason: "this row will not be imported, so there is nobody to link to " + edge.ToID,
			})
			continue
		}
		resolved, err := writers.resolveEmployer(ctx, edge.ToID)
		if err != nil {
			return migration.Report{}, err
		}
		if resolved.found {
			resolvable++
			continue
		}
		report.AssociationsSkipped = append(report.AssociationsSkipped, migration.SkippedAssoc{
			From:   edge.FromType + "/" + edge.FromID,
			To:     edge.ToType + "/" + edge.ToID,
			Reason: resolved.reason,
		})
	}
	report.Associations = resolvable
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
				p.absent = append(p.absent, row.ExternalID)
				// Disclosed as a skip rather than counted as a create: the
				// commit will refuse this row, and the report exists to say so
				// before a human approves it.
				p.skipped = append(p.skipped, migration.SkippedRow{
					ExternalID: row.ExternalID,
					Line:       row.Line,
					Reason:     refusal,
				})
			case predictCollidesSkipped:
				p.absent = append(p.absent, row.ExternalID)
				// The run asked to skip duplicates, so this row is a skip and
				// the preview says so — the commit must not be the first place
				// a person learns the row did not land.
				p.duplicates++
				p.skipped = append(p.skipped, migration.SkippedRow{
					ExternalID: row.ExternalID,
					Line:       row.Line,
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
					Line:       row.Line,
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
	// absent are the rows the commit will NOT land — refused outright, or a
	// duplicate this run asked to skip. Their employer links cannot be written,
	// so the preview must not count them as resolvable.
	absent []string
	// duplicates counts the same rows. It is reported to the human as its own
	// number ("100 companies, 94 duplicates") and never summed with the four.
	duplicates int
}
