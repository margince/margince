// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the dry run TELLS a colleague before they approve it.
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
		report.Objects[i].Warnings = append(report.Objects[i].Warnings, p.warnings...)
		report.Objects[i].WillDuplicate += p.duplicates
	}
	return withPredictedLinks(ctx, source, writers, report, p.absent)
}

// withPredictedLinks adds, before anything is written, how many of one part's
// employer links will actually find a company. Parts add to one count, which
// predictEveryPart starts at zero.
//
// The engine's own dry run counts edges OFFERED — it has no writer, so it cannot
// resolve an endpoint. That number alone would tell a contact approving a file of
// 12,000 contacts that 12,000 links are coming, when the truthful answer might be
// 3,000 and 9,000 companies nobody has imported yet. So the resolvable half is
// computed here through the SAME resolver the commit will use, and the names that
// resolve to nothing are listed rather than counted — the answer a contact needs
// is WHICH company was not found, because that is the row they have to go fix.
//
// Only the COMPANY end is resolved. At dry-run time no contact has landed, so the
// contact end would answer "not imported" for every row, and predicting that would
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
	// A row the commit will refuse lands no contact, so its employer link cannot
	// be written either. Counting it as resolvable would promise a link whose
	// contact is never there — preview says "linked", commit says "nobody to link
	// to", which is the disagreement this whole pass exists to prevent.
	willNotLand := make(map[string]bool, len(absent))
	for _, id := range absent {
		willNotLand[id] = true
	}
	for _, edge := range edges {
		if willNotLand[edge.FromID] {
			report.AssociationsSkipped = append(report.AssociationsSkipped, migration.SkippedAssoc{
				From:   edge.FromType + "/" + edge.FromID,
				To:     edge.ToType + "/" + edge.ToID,
				Reason: "this row will not be imported, so there is nobody to link to " + edge.ToID,
			})
			continue
		}
		reason, err := writers.predictLink(ctx, edge)
		if err != nil {
			return migration.Report{}, err
		}
		if reason == "" {
			report.Associations++
			continue
		}
		report.AssociationsSkipped = append(report.AssociationsSkipped, migration.SkippedAssoc{
			From:   edge.FromType + "/" + edge.FromID,
			To:     edge.ToType + "/" + edge.ToID,
			Reason: reason,
		})
	}
	return report, nil
}

// predictLink answers why an edge will not find its company, or "" when it
// will. A company_ref goes through the run's resolver, which counts a company of
// this upload. A company name goes through the name match the commit runs.
func (w *csvWriters) predictLink(ctx context.Context, edge migration.Assoc) (string, error) {
	if edge.ToType == migration.AssocTargetCompanyRef {
		answer, err := w.links.resolveCompany(ctx, edge.ToID)
		if err != nil || answer.resolves() {
			return "", err
		}
		return answer.Reason, nil
	}
	resolved, err := w.resolveEmployer(ctx, edge.ToID)
	if err != nil || resolved.found {
		return "", err
	}
	return resolved.reason, nil
}

// predictPages walks the source and tallies what the commit would do with each
// row, page by page, the same way the engine will walk it. A row predicted to
// land is marked pending, so a later file's link to it resolves.
func predictPages(ctx context.Context, source *migration.CSVSource, writers *csvWriters, object string) (p prediction, err error) {
	for offset := 0; ; offset += importPredictPage {
		rows, err := source.Rows(ctx, object, offset, importPredictPage)
		if err != nil {
			return prediction{}, err
		}
		for _, row := range rows {
			outcome, problem, err := writers.predictRow(ctx, row)
			if err != nil {
				return prediction{}, err
			}
			if p.tally(object, row, outcome, problem) {
				writers.links.markPending(object, row.ExternalID)
				if object == migration.ObjectCompany {
					fields := textFields(row.Fields)
					creates := outcome == predictCreate || outcome == predictCollides
					writers.links.markNamesPending(creates, fields[fieldDisplayName], fields[fieldLegalName])
				}
			}
		}
		if len(rows) < importPredictPage {
			return p, nil
		}
	}
}

// tally counts one row's predicted outcome and answers whether the row lands.
func (p *prediction) tally(object string, row migration.Row, outcome predictedOutcome, problem *migration.SkippedRow) bool {
	collision, skipReason := collisionWordingFor(object)
	switch outcome {
	case predictUnwritable:
		p.absent = append(p.absent, row.ExternalID)
		// Disclosed as a skip rather than counted as a create. The commit will
		// refuse this row, and the report says so before a human approves it.
		p.skipped = append(p.skipped, *problem)
		return false
	case predictCollidesSkipped:
		p.absent = append(p.absent, row.ExternalID)
		// The run asked to skip duplicates, so the preview reports this row as a
		// skip. The commit must not be the first place a contact learns of it.
		p.duplicates++
		p.skipped = append(p.skipped, migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line, Reason: skipReason})
		return false
	case predictCollides:
		p.duplicates++
		// Still a create (the commit lands it and files a review pair), so it is
		// counted as one, with the warning kept apart. The four counts sum to
		// rows_read, so a row counted as created and skipped reports two outcomes.
		p.created++
		p.collisions = append(p.collisions, migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line, Reason: collision})
	case predictUpdate:
		p.updated++
	case predictUnchanged:
		p.unchanged++
	case predictCreate:
		p.created++
	}
	if problem != nil {
		p.warnings = append(p.warnings, *problem)
	}
	return true
}

// collisionDisclosure is what the report says about a row naming a company the
// CRM already holds. It is not a refusal: the commit creates the row and files
// the pair on the dedupe review queue, exactly as a manual create would. The
// disclosure exists so a reader reading "create 3" before approving is not
// surprised by a duplicate afterwards.
const collisionDisclosure = "a company of this name is already in the CRM; " +
	"importing this row creates a second one and files the pair for review"

// duplicateSkipReason is what the report says about a row the run chose to
// leave alone, under `on_duplicate: skip`.
const duplicateSkipReason = "a company of this name is already in the CRM, and this run was asked to skip duplicates"

// The same two sentences for a contact run, which must not tell its reader that a
// company was found.
//
// The disclosure is not merely the company one with a noun swapped. An address
// the estate already holds is REFUSED by the store — uq_contact_email_dedupe is a
// real key, where a company name is not — so promising "a second one is created"
// would be false for the very case a reader is most likely to hit.
const contactCollisionDisclosure = "someone matching this row is already in the CRM; " +
	"a row naming an address already held is refused, and a near match creates and files the pair for review"

const contactDuplicateSkipReason = "someone matching this row is already in the CRM, and this run was asked to skip duplicates"

// collisionWordingFor names the sentences that match what this run imports.
func collisionWordingFor(object string) (disclosure, skipReason string) {
	if object == migration.ObjectContact {
		return contactCollisionDisclosure, contactDuplicateSkipReason
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
	// warnings are rows that land with something reported about them, such
	// as a dropped link.
	warnings []migration.SkippedRow
	// duplicates counts the same rows. It is reported to the human as its own
	// number ("100 companies, 94 duplicates") and never summed with the four.
	duplicates int
}
