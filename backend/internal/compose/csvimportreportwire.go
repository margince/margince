// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading a run's report back onto the wire: what the import DID, in the file's
// own terms.
//
// Split from the request half of the wire because the two answer opposite
// questions — one takes a caller's mapping apart and refuses what the estate
// cannot receive, the other puts a finished run's outcome into a shape a contact
// reads. They shared a file until it passed the length cap, and nothing but
// history held them together.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/importwire"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func toContractImportReport(run migration.Run) crmcontracts.ImportRunReport {
	out := crmcontracts.ImportRunReport{
		RunId:  openapi_types.UUID(run.ID),
		Status: crmcontracts.ImportRunStatus(run.Status),
		Issues: []crmcontracts.ImportRowIssue{},
	}
	if run.Mapping != nil {
		out.SourceKeyUsed = run.Mapping.SourceKey
		if fields := run.Mapping.Fields; fields != nil {
			out.Mapping = &fields
		}
	}

	// Predicted counts and actual ones are never summed: a finished run reports
	// what it did, an awaiting one what it will do. Adding them would report
	// twice the rows the file holds — and the stored report DOES carry both,
	// because a run's own report is merged into the dry run's so a resumed
	// attempt keeps what the earlier one already achieved.
	committed := hasCommitted(run.Status)
	// Warnings fold apart from skips, so a row's warning is never taken for
	// that row's skip.
	seen, seenWarnings := map[string]bool{}, map[string]bool{}
	duplicates := 0
	parts := make([]crmcontracts.ImportPartReport, 0, len(run.Report.Objects))
	reached := rowsReachedOf(run)
	for i, o := range run.Report.Objects {
		rowsRead := reached[i]
		d := importDispositionOf(o, committed)
		if committed {
			d = withoutStaleSkips(d, rowsRead, run.Report.Walks())
		}
		out.RowsRead += rowsRead
		out.Disposition.Created += d.Created
		out.Disposition.Updated += d.Updated
		out.Disposition.Skipped += d.Skipped
		out.Disposition.Unchanged += d.Unchanged
		duplicates += *d.Duplicates
		parts = append(parts, crmcontracts.ImportPartReport{
			File: partFileOf(run, o), Object: crmcontracts.ImportObject(o.Object),
			RowsRead: rowsRead, Disposition: d,
			ClosedWon: countOrAbsent(o.ClosedWon), ClosedLost: countOrAbsent(o.ClosedLost),
		})
		importwire.AppendIssues(&out.Issues, seen, importwire.SkipKey, o, o.Skipped)
		// A collision is reported the same way a skip is — by line, in the
		// file's own terms — but it does NOT touch Disposition.Skipped: the
		// row is counted in Created, and adding it here too would report two
		// outcomes for one row and leave `unchanged` short by the difference.
		importwire.AppendIssues(&out.Issues, seen, importwire.IssueKey, o, o.Collisions)
		// A warning is a row that lands with something to say about it, so it
		// is named the same way and counted nowhere.
		importwire.AppendIssues(&out.Issues, seenWarnings, importwire.WarningKey, o, o.Warnings)
	}
	out.Parts = &parts
	out.Issues = collapseUnknownOwners(out.Issues)
	retell(out.Issues, runTense{written: committed, tagsCreated: committed && run.Error != importTagsFailedCause})

	// Reported even when zero: "0 duplicates" is the answer to the question a
	// contact is asking before they approve, and an omitted field reads as "not
	// checked" rather than "none found".
	out.Disposition.Duplicates = &duplicates
	out.Links = linksOf(run.Report, committed)

	// Present once the run has been undone, not while undoing — a
	// still-in-progress or interrupted reversal's partial counts are the
	// run's own internal resume state, not a finished outcome to report.
	if run.UndoReport != nil && run.Status == migration.StatusUndone {
		out.Undo = toContractUndoReport(run.ID, run.Status, *run.UndoReport)
	}
	return out
}

// hasCommitted is whether a run in this status has written, so its stored
// counts say what it did and not what its test predicted.
func hasCommitted(status string) bool {
	switch status {
	case migration.StatusComplete, migration.StatusFailed, migration.StatusUndoing, migration.StatusUndone:
		return true
	}
	return false
}

// rowsWrittenOf is how many rows of one object the run created or updated,
// nil before it has written any.
func rowsWrittenOf(run migration.Run, object string) *int {
	if run.Report == nil || !hasCommitted(run.Status) {
		return nil
	}
	for _, o := range run.Report.Objects {
		if o.Object == object {
			d := importDispositionOf(o, true)
			return new(d.Created + d.Updated)
		}
	}
	return nil
}

// rowsReachedOf is how many rows of each reported object the run read. A
// failed commit read only up to its checkpoint, in import order, and the rows
// past it are none of the four outcomes.
func rowsReachedOf(run migration.Run) []int {
	reached := make([]int, len(run.Report.Objects))
	left := run.Checkpoint
	for i, o := range run.Report.Objects {
		reached[i] = o.MirrorCount
		if run.Status == migration.StatusFailed {
			reached[i] = min(o.MirrorCount, left)
			left -= reached[i]
		}
	}
	return reached
}

// runTense is what a run has done by the time its report is read.
type runTense struct {
	// written: the run has written, so a dropped link was left out, not will be.
	written bool
	// tagsCreated: the commit created every new tag before its first row.
	tagsCreated bool
}

// retell words each issue in the tense of its run.
func retell(issues []crmcontracts.ImportRowIssue, tense runTense) {
	for i := range issues {
		why, reworded := retold(issues[i], tense)
		if !reworded {
			continue
		}
		issues[i].Reason = why.English()
		issues[i].ReasonCode = importwire.ReasonCodeOrAbsent(why.Code)
		issues[i].Params = importwire.ParamsOrAbsent(why.Params)
	}
}

// retold is the reason an issue takes in the tense of its run, and whether
// that is another reason than the one it carries.
func retold(issue crmcontracts.ImportRowIssue, tense runTense) (migration.Reason, bool) {
	if kind := issue.Kind; tense.tagsCreated && kind != nil && string(*kind) == migration.ProblemNewTag {
		return createdTagWhy, true
	}
	if issue.ReasonCode == nil {
		return migration.Reason{}, false
	}
	stored := migration.Reason{Code: migration.ReasonCode(*issue.ReasonCode)}
	if issue.Params != nil {
		stored.Params = *issue.Params
	}
	told := stored.Told(tense.written)
	return told, told.Code != stored.Code
}

// walksWithoutAResume is how many times an ordinary committed run is walked:
// the dry run, then the commit. A report folding more than these has been
// resumed, and a resume is the one cause of surplus that must not be taken out
// of `skipped`.
const walksWithoutAResume = 2

// importDispositionOf is one object's four counts, predicted before approval and
// observed after it. Totals and parts both add from here.
func importDispositionOf(o migration.ObjectReport, committed bool) crmcontracts.ImportRunDisposition {
	d := crmcontracts.ImportRunDisposition{Created: o.WillCreate, Updated: o.WillUpdate}
	// After the commit its own counts stand. The estate moves between preview
	// and approval, so a finished report must say what happened, not what was
	// expected.
	duplicates := o.WillDuplicate
	if committed {
		d.Created, d.Updated, duplicates = o.Created, o.Updated, o.Duplicated
	}
	d.Duplicates = &duplicates
	d.Skipped = importwire.DistinctIssues(o, o.Skipped)
	d.Unchanged = unchangedOf(d, o.MirrorCount)
	return d
}

// withoutStaleSkips takes a committed object's surplus out of `skipped` when
// the run was never resumed.
//
// A dry-run skip can commit as a create and leave a stale skip beside it.
// A resume can also count a row twice, and then the skip is real. Only the
// walk count tells them apart: two walks (dry run, commit) is no resume.
func withoutStaleSkips(d crmcontracts.ImportRunDisposition, rowsRead, walks int) crmcontracts.ImportRunDisposition {
	surplus := d.Created + d.Updated + d.Skipped - rowsRead
	if surplus > 0 && walks <= walksWithoutAResume {
		d.Skipped = max(d.Skipped-surplus, 0)
	}
	d.Unchanged = unchangedOf(d, rowsRead)
	return d
}

// unchangedOf derives the rows read and neither created, updated nor skipped.
// The stored figure adds the dry run's count to the commit's. Carrying it would
// break the rule that the four sum to the rows read.
func unchangedOf(d crmcontracts.ImportRunDisposition, rowsRead int) int {
	return max(rowsRead-d.Created-d.Updated-d.Skipped, 0)
}

// partFileOf names the file an object's rows came from: its ZIP entry, or the
// uploaded file's own name for a single CSV.
func partFileOf(run migration.Run, o migration.ObjectReport) string {
	if o.File == "" && run.Mapping != nil {
		return run.Mapping.FileName
	}
	return o.File
}

// linksOf renders the connections a run makes, for the half of the report that
// is not about rows.
//
// The stored count means two different things either side of approval, which is
// the engine's own shape rather than a quirk to smooth over. A dry run cannot
// resolve an endpoint — it writes nothing and reads nothing about the other end
// — so what it can honestly say is how many rows NAMED an employer. After the
// commit the same field counts the edges actually written. Reporting them under
// one number would let a preview promising 12,000 links be answered by 3,000
// applied without either figure ever being wrong.
//
// Always non-nil for the same reason `duplicates` is: "0 links" answers the
// question, an absent field reads as "not checked".
func linksOf(rep *migration.Report, committed bool) *crmcontracts.ImportRunLinks {
	links := crmcontracts.ImportRunLinks{}
	if rep == nil {
		empty := []crmcontracts.ImportUnresolvedLink{}
		links.Unresolved = &empty
		return &links
	}
	// One stored count, two meanings, decided by which side of approval the run
	// is on. Before it, the engine has no writer and cannot resolve an endpoint,
	// so the number is what the file asks for that CAN be linked. After it, the
	// same field holds what was actually written. Reporting both under one name
	// would let a preview promising 12,000 links be answered by 3,000 applied
	// without either figure ever having been wrong.
	if committed {
		links.Applied = rep.Associations
	}
	links.Offered = rep.Associations
	unresolved := make([]crmcontracts.ImportUnresolvedLink, 0, len(rep.AssociationsSkipped))
	for _, s := range rep.AssociationsSkipped {
		// The link of a row that did not land is that row's skip, already
		// reported as one, and no link was there to make.
		if !s.RowSkipped && !linkOfAnUnlandedRow[s.Code] {
			links.Offered++
		}
		unresolved = append(unresolved, crmcontracts.ImportUnresolvedLink{
			From: s.From, To: s.To, Reason: s.Reason,
			ReasonCode: importwire.ReasonCodeOrAbsent(s.Code), Params: importwire.ParamsOrAbsent(s.Params),
		})
	}
	links.Unresolved = &unresolved
	return &links
}

// linkOfAnUnlandedRow are the reasons a link names when its own row is not
// imported: before the commit and after it.
var linkOfAnUnlandedRow = map[migration.ReasonCode]bool{
	migration.ReasonLinkRowNotImported: true, migration.ReasonLinkContactNotImported: true,
}

// undoErroredRow is the generated ImportUndoReport.Errored item, declared here
// for the slice and the literals that fill it.
type undoErroredRow = struct {
	Id         openapi_types.UUID             `json:"id"` //nolint:staticcheck // matches the generated item shape
	Object     crmcontracts.ImportObject      `json:"object"`
	Reason     string                         `json:"reason"`
	ReasonCode *crmcontracts.ImportReasonCode `json:"reason_code,omitempty"`
}

// undoKeptRow is the generated ImportUndoReport.Kept item, declared here for
// the slice and the literals that fill it.
type undoKeptRow = struct {
	Id         openapi_types.UUID             `json:"id"` //nolint:staticcheck // matches the generated item shape
	Object     crmcontracts.ImportObject      `json:"object"`
	Params     *map[string]string             `json:"params,omitempty"`
	Reason     *string                        `json:"reason,omitempty"`
	ReasonCode *crmcontracts.ImportReasonCode `json:"reason_code,omitempty"`
}

// toContractUndoReport renders the reversal outcome (IEM-WIRE-9).
func toContractUndoReport(id migration.RunID, status string, rep migration.UndoReport) *crmcontracts.ImportUndoReport {
	kept := make([]undoKeptRow, 0, len(rep.Kept))
	for _, k := range rep.Kept {
		kept = append(kept, undoKeptRow{
			Id: openapi_types.UUID(k.ID), Object: crmcontracts.ImportObject(k.Object),
			Reason: importwire.TextOrAbsent(k.Reason), ReasonCode: importwire.ReasonCodeOrAbsent(k.Code), Params: importwire.ParamsOrAbsent(k.Params),
		})
	}
	errored := make([]undoErroredRow, 0, len(rep.Errored))
	for _, e := range rep.Errored {
		errored = append(errored, undoErroredRow{
			Id: openapi_types.UUID(e.ID), Object: crmcontracts.ImportObject(e.Object),
			Reason: e.Reason, ReasonCode: importwire.ReasonCodeOrAbsent(e.Code),
		})
	}
	out := &crmcontracts.ImportUndoReport{
		RunId:         openapi_types.UUID(id),
		Status:        crmcontracts.ImportRunStatus(status),
		ReversedCount: rep.ReversedCount,
		Kept:          kept,
		Errored:       errored,
	}
	// Omitted when the run created everything it touched, which is what an
	// absent count means on the wire.
	if rep.UpdatesNotReversed > 0 {
		out.UpdatesNotReversed = &rep.UpdatesNotReversed
	}
	return out
}

// countOrAbsent is a count the part reports only when it is not zero, so a part
// that holds no closed deal says nothing about them.
func countOrAbsent(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
