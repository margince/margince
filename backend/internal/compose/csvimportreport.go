// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the dry run TELLS a person before they approve it.
//
// The engine's own report counts rows it would touch; this walks the file a
// second time and asks the writers what each row would actually do — created,
// updated, unchanged, refused, or already here. That second pass is the whole
// value of a preview: an approval is a decision about what the report said, so a
// report that says "create 100" for a file of 94 companies already held is not a
// smaller truth but a different one.
//
// The four counts sum to rows_read and no row is counted twice among them.
// `duplicates` sits outside that sum deliberately — it is a disclosure about
// rows already counted elsewhere, not a fifth outcome.

import (
	"context"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/migration"
)

// refinePrediction replaces the engine's create/update split with one the
// report can honestly show a human.
//
// The engine classifies from Writers.Exists alone, so every row that already
// landed counts as an update — including the ones whose values are identical,
// which the commit will not rewrite. A dry run whose whole job is to say what
// WILL happen may not overstate it by the size of the customer's re-upload, so
// each row is compared here exactly as the commit will compare it.
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
	for offset := 0; ; offset += importPredictPage {
		rows, err := source.Rows(ctx, object, offset, importPredictPage)
		if err != nil {
			return prediction{}, err
		}
		for _, row := range rows {
			outcome, err := writers.Predict(ctx, row)
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
					Reason:     unwritableReason(object, textFields(row.Fields)),
				})
			case predictCollidesSkipped:
				// The run asked to skip duplicates, so this row is a skip and
				// the preview says so — the commit must not be the first place
				// a person learns the row did not land.
				p.duplicates++
				p.skipped = append(p.skipped, migration.SkippedRow{
					ExternalID: row.ExternalID,
					Reason:     duplicateSkipReason,
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
					Reason:     collisionDisclosure,
				})
			case predictCollidesUpdate:
				// Counted as an UPDATE, because that is the effect: the file's
				// values land on the record already here. It is a duplicate too,
				// and both numbers are told — `duplicates` is the disclosure a
				// person weighs, `updated` is one of the four that sum to
				// rows_read, and no row is ever counted twice among those four.
				p.duplicates++
				p.updated++
				p.collisions = append(p.collisions, migration.SkippedRow{
					ExternalID: row.ExternalID,
					Reason:     updateDisclosure,
				})
			case predictCollidesUnchanged:
				// Matched, and the file says nothing new. Counted as unchanged
				// for the reason predictUnchanged is: an update that writes
				// nothing must not appear as work in the report or the audit log.
				p.duplicates++
				p.unchanged++
			case predictCollidesUnfit:
				// The ladder only reached fuzzy_review, so the run will not write
				// on the guess. Reported as a skip with its own reason: "we think
				// this might be the same company, and we are not willing to
				// overwrite on a maybe" is a different message from "you asked us
				// to skip duplicates", and the person holding the file is the one
				// who can resolve it.
				p.duplicates++
				p.skipped = append(p.skipped, migration.SkippedRow{
					ExternalID: row.ExternalID,
					Reason:     fuzzyUpdateSkipReason,
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

// opaqueSkipReason is what the report says about a row skipped for colliding
// with a record the CALLER MAY NOT SEE.
//
// It says the row was left alone and stops there. duplicateSkipReason would say
// a company of that name is already in the CRM, which is exactly the fact a
// caller must not learn about a colleague's owner-private capture — and a
// finished run's report is readable on the import_run grant alone, so putting it
// there would move the existence oracle from the preview to the finished run
// rather than closing it.
//
// The row is still skipped. The DECISION turns on the record existing, which is
// the importer's business to act on and not this caller's to be told.
const opaqueSkipReason = "this row was left alone; it could not be imported as a new company"

// updateDisclosure is what the report says about a row that will be written onto
// the company already here, under `on_duplicate: update`. Still a disclosure
// rather than a warning: the run asked for exactly this, and the person
// approving should see which rows are edits rather than additions.
const updateDisclosure = "a company of this name is already in the CRM; " +
	"importing this row updates it rather than creating a second one"

// fuzzyUpdateSkipReason is what the report says about a row an update run will
// not write onto, because the names are only SIMILAR.
//
// Its own reason rather than duplicateSkipReason, because the two ask different
// things of the reader. A skipped duplicate needs no action — the run was told
// to leave it. This one is unfinished business: two records that may or may not
// be one company, which only the file's author can settle, and which the run
// refused to settle by overwriting.
const fuzzyUpdateSkipReason = "a company with a similar name is already in the CRM, and this run " +
	"will not overwrite a record on a name match alone — merge them in the app, or make the names match exactly"

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

// duplicatePolicies names what on_duplicate accepts, read off the contract's own
// enum rather than typed into the refusal.
//
// The message used to say "create or skip" in prose, which went stale the moment
// `update` landed — a caller asking for a real policy would have been told it did
// not exist. A refusal that names the vocabulary has to be derived from the
// vocabulary.
func duplicatePolicies() []string {
	all := []crmcontracts.ImportOnDuplicate{
		crmcontracts.Create, crmcontracts.Skip, crmcontracts.Update,
	}
	out := make([]string, 0, len(all))
	for _, p := range all {
		out = append(out, string(p))
	}
	return out
}
