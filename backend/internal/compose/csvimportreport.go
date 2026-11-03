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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func refinePrediction(ctx context.Context, source *migration.CSVSource, writers *csvWriters, object string, report migration.Report) (migration.Report, error) {
	p, err := predictPages(ctx, source, writers, object)
	if err != nil {
		return migration.Report{}, err
	}
	if report.ReviewPlan == nil {
		report.ReviewPlan = &migration.ReviewPlan{Version: 1, Complete: true}
	}
	if !p.review.Complete {
		report.ReviewPlan.Complete, report.ReviewPlan.Rows = false, nil
	}
	for _, row := range p.review.Rows {
		report.ReviewPlan.Add(row)
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
		report.Objects[i].ClosedWon += p.closedWon
		report.Objects[i].ClosedLost += p.closedLost
	}
	return withPredictedLinks(ctx, source, writers, report, p.absent)
}

// withPredictedLinks adds how many of one part's employer links will find a
// company. Parts add to one count, which predictEveryPart starts at zero.
// The engine's dry run has no writer, so it counts links offered, not links
// that resolve. This uses the commit's resolver and lists each unfound name,
// the row a reader has to fix. Only the company end is resolved: no contact
// has landed yet, so the contact end would read "not imported" on every row.
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
		why, err := writers.predictLink(ctx, edge)
		if err != nil {
			return migration.Report{}, err
		}
		if why.Code == "" && willNotLand[edge.FromID] {
			why = migration.Why(migration.ReasonLinkRowNotImported, "name", edge.ToID)
		}
		if why.Code == "" {
			report.Associations++
			continue
		}
		report.AssociationsSkipped = append(report.AssociationsSkipped, skippedAssocFor(edge, why))
	}
	return report, nil
}

// skippedAssocFor is the report's line for an edge left unapplied for why.
func skippedAssocFor(edge migration.Assoc, why migration.Reason) migration.SkippedAssoc {
	return migration.SkippedAssoc{
		From: edge.FromType + "/" + edge.FromID, To: edge.ToType + "/" + edge.ToID,
		Reason: why.English(), Code: why.Code, Params: why.Params,
	}
}

// predictLink answers why an edge will not find its company, or the zero
// reason when it will. A company_ref goes through the run's resolver, which counts a company of
// this upload. A company name goes through the name match the commit runs.
func (w *csvWriters) predictLink(ctx context.Context, edge migration.Assoc) (migration.Reason, error) {
	if edge.ToType == migration.AssocTargetCompanyRef {
		answer, err := w.links.resolveCompany(ctx, edge.ToID)
		if err != nil || answer.resolves() {
			return migration.Reason{}, err
		}
		return answer.Why, nil
	}
	resolved, err := w.resolveEmployer(ctx, edge.ToID)
	if err != nil || resolved.found || resolved.why.Code == migration.ReasonEmployerNotFound {
		return migration.Reason{}, err
	}
	return resolved.why, nil
}

// predictPages walks the source and tallies what the commit would do with each
// row, page by page, the same way the engine will walk it. A row predicted to
// land is marked pending, so a later file's link to it resolves.
func predictPages(ctx context.Context, source *migration.CSVSource, writers *csvWriters, object string) (p prediction, err error) {
	p.review = migration.ReviewPlan{Version: 1, Complete: true}
	for offset := 0; ; offset += importPredictPage {
		rows, err := source.Rows(ctx, object, offset, importPredictPage)
		if err != nil {
			return prediction{}, err
		}
		if err := writers.readPageEmails(ctx, rows); err != nil {
			return prediction{}, err
		}
		for _, row := range rows {
			outcome, problem, err := writers.predictRow(ctx, row)
			if err != nil {
				return prediction{}, err
			}
			if p.review.Complete {
				review, err := writers.reviewRow(ctx, row, outcome)
				if err != nil {
					return prediction{}, err
				}
				p.review.Add(review)
			}
			if !p.tally(object, row, outcome, problem) {
				writers.links.markRefused(object, row.ExternalID)
				continue
			}
			p.countClosedDeal(writers.dealStages, object, row, outcome)
			writers.markPredictedRow(row, outcome)
		}
		if len(rows) < importPredictPage {
			return p, nil
		}
	}
}

// tally counts one row's predicted outcome and answers whether the row lands.
func (p *prediction) tally(object string, row migration.Row, outcome predictedOutcome, problem *migration.SkippedRow) bool {
	collision, skipWhy := collisionWordingFor(object)
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
		p.skipped = append(p.skipped, migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line}.Because(skipWhy))
		return false
	case predictCollides:
		p.duplicates++
		// Still a create (the commit lands it and files a review pair), so it is
		// counted as one, with the warning kept apart. The four counts sum to
		// rows_read, so a row counted as created and skipped reports two outcomes.
		p.created++
		p.collisions = append(p.collisions, migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line}.Because(collision))
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

// countClosedDeal tallies a deal that will be created on a won or lost stage,
// which the commit writes as closed history.
func (p *prediction) countClosedDeal(stages dealStages, object string, row migration.Row, outcome predictedOutcome) {
	if object != migration.ObjectDeal || (outcome != predictCreate && outcome != predictCollides) {
		return
	}
	choice, named, _ := stages.stageOf(textFields(row.Fields))
	switch {
	case !named:
	case choice.semantic == crmcontracts.StageSemanticWon:
		p.closedWon++
	case choice.semantic == crmcontracts.StageSemanticLost:
		p.closedLost++
	}
}

// collisionWordingFor names two reasons. The disclosure is for a row that lands
// beside a record the estate holds. The skip is for a row the run leaves alone.
//
// The contact disclosure is not the company one with a noun swapped. The store
// refuses an address it already holds, where a company name is no key. Promising
// "a second one is created" would be false for the case a reader most often hits.
func collisionWordingFor(object string) (disclosure, skip migration.Reason) {
	if object == migration.ObjectContact {
		return contactCollisionWhy, contactDuplicateSkipWhy
	}
	return companyCollisionWhy, companyDuplicateSkipWhy
}

// prediction is what one walk of the source concluded: the three outcomes a
// commit can have, plus the rows it will refuse outright.
type prediction struct {
	review    migration.ReviewPlan
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
	// closedWon and closedLost count the new deals that land already closed.
	closedWon, closedLost int
}
