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
	"fmt"
	"regexp"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

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
	committed := run.Status == migration.StatusComplete || run.Status == migration.StatusFailed ||
		run.Status == migration.StatusUndoing || run.Status == migration.StatusUndone
	// Warnings fold apart from skips, so a row's warning is never taken for
	// that row's skip.
	seen, seenWarnings := map[string]bool{}, map[string]bool{}
	duplicates := 0
	parts := make([]crmcontracts.ImportPartReport, 0, len(run.Report.Objects))
	for _, o := range run.Report.Objects {
		d := importDispositionOf(o, committed)
		if committed {
			d = withoutStaleSkips(d, o.MirrorCount, run.Report.Walks())
		}
		out.RowsRead += o.MirrorCount
		out.Disposition.Created += d.Created
		out.Disposition.Updated += d.Updated
		out.Disposition.Skipped += d.Skipped
		out.Disposition.Unchanged += d.Unchanged
		duplicates += *d.Duplicates
		parts = append(parts, crmcontracts.ImportPartReport{
			File: partFileOf(run, o), Object: crmcontracts.ImportObject(o.Object),
			RowsRead: o.MirrorCount, Disposition: d,
		})
		appendIssues(&out.Issues, seen, issueKey, o, o.Skipped)
		// A collision is reported the same way a skip is — by line, in the
		// file's own terms — but it does NOT touch Disposition.Skipped: the
		// row is counted in Created, and adding it here too would report two
		// outcomes for one row and leave `unchanged` short by the difference.
		appendIssues(&out.Issues, seen, issueKey, o, o.Collisions)
		// A warning is a row that lands with something to say about it, so it
		// is named the same way and counted nowhere.
		appendIssues(&out.Issues, seenWarnings, warningKey, o, o.Warnings)
	}
	out.Parts = &parts

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
	d.Skipped = distinctIssues(o, o.Skipped)
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
	// Offered is the whole ask either way: what landed, plus what named a company
	// the run could not find.
	links.Offered = rep.Associations + len(rep.AssociationsSkipped)
	unresolved := make([]crmcontracts.ImportUnresolvedLink, 0, len(rep.AssociationsSkipped))
	for _, s := range rep.AssociationsSkipped {
		unresolved = append(unresolved, crmcontracts.ImportUnresolvedLink{
			From: s.From, To: s.To, Reason: s.Reason,
		})
	}
	links.Unresolved = &unresolved
	return &links
}

// lineOf is the file line a skip names, from the row itself.
//
// The fallback is for reports STORED before the line was carried. Their JSON
// has no `line` field, so it decodes as zero — and for the skips the source
// disclosed, the id still spells "line N", which is where the line used to be
// read from for every skip. Deriving it only when the carried one is absent
// keeps those reports readable through a rollout without putting the old
// parse back in the path: a row that carries its own id answers 0 there, which
// is the defect this replaced.
func lineOf(skip migration.SkippedRow) int {
	if skip.Line != 0 {
		return skip.Line
	}
	match := disclosedLine.FindStringSubmatch(skip.ExternalID)
	if match == nil {
		return 0
	}
	line, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return line
}

// appendIssues adds one object's refused rows to the report's issues and
// answers how many were new, so the caller can count the ones that are skips.
//
// The same row skipped by the dry run and again by the commit is ONE row the
// human must go fix. Keyed on the EXTERNAL ID, which is what identifies a row.
// Keyed on the line instead, every skipped row in a file that carries its own
// key column collapsed onto the first — the line was derived from the id's text
// shape back then and answered 0 for all of them — so two refused rows were
// reported as one skip and one phantom `unchanged`, and the four counts summed
// to less than rows_read. The line is carried now and would no longer collide,
// but the id is still the right key: it is what a row IS, where a line is where
// it happened to sit.
//
// The LINE joins it, because the id alone is not always this source's. A row the
// source could not identify is disclosed as `line 7`, and a file whose key
// column legitimately holds the text "line 7" would collide with it and lose a
// row from the report. The pair separates them, and it folds the dry run's skip
// onto the commit's exactly as before: one row keeps one line across both.
func appendIssues(
	issues *[]crmcontracts.ImportRowIssue, seen map[string]bool,
	keyOf func(migration.ObjectReport, migration.SkippedRow) string, o migration.ObjectReport, rows []migration.SkippedRow,
) {
	for _, row := range rows {
		key := keyOf(o, row)
		if seen[key] {
			continue
		}
		seen[key] = true
		*issues = append(*issues, crmcontracts.ImportRowIssue{
			Line:   lineOrAbsent(lineOf(row)),
			Reason: row.Reason,
			File:   textOrAbsent(o.File),
			Column: textOrAbsent(row.Column),
			Kind:   kindOrAbsent(row.Kind),
			Value:  textOrAbsent(row.Value),
		})
	}
}

// distinctIssues counts the rows appendIssues names once each.
func distinctIssues(o migration.ObjectReport, rows []migration.SkippedRow) int {
	seen := map[string]bool{}
	for _, row := range rows {
		seen[issueKey(o, row)] = true
	}
	return len(seen)
}

// issueKey is what makes two reported rows one. The object joins it because
// two files of a ZIP may each have a row `line 7`.
func issueKey(o migration.ObjectReport, row migration.SkippedRow) string {
	return o.Object + "\x00" + row.ExternalID + "\x00" + strconv.Itoa(row.Line)
}

// warningKey is issueKey with the kind and value. One row may carry several
// warnings, and a part's new tags belong to no row at all.
func warningKey(o migration.ObjectReport, row migration.SkippedRow) string {
	return issueKey(o, row) + "\x00" + row.Kind + "\x00" + row.Value
}

// lineOrAbsent leaves out the line of a problem that names no row, such as a
// tag the whole file refers to.
func lineOrAbsent(line int) *int {
	if line == 0 {
		return nil
	}
	return &line
}

func textOrAbsent(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func kindOrAbsent(kind string) *crmcontracts.ImportProblemKind {
	if kind == "" {
		return nil
	}
	k := crmcontracts.ImportProblemKind(kind)
	return &k
}

// disclosedLine is the WHOLE id the source writes for a row it could not
// identify, anchored at both ends.
//
// Anchored because the id is not always one of ours: a mirror source's rows
// carry the incumbent's id, and a prefix match would read "line 7 of the
// export" as line 7. That was already true of the parse this replaces; what is
// left after the anchors is an incumbent id spelled exactly `line 7`, on a
// report stored before the line was carried.
var disclosedLine = regexp.MustCompile(`^line ([0-9]+)$`)

// toContractUndoReport renders the reversal outcome (IEM-WIRE-9).
func toContractUndoReport(id migration.RunID, status string, rep migration.UndoReport) *crmcontracts.ImportUndoReport {
	kept := make([]struct {
		Id     openapi_types.UUID        `json:"id"` //nolint:staticcheck // matches the generated ImportUndoReport.Kept item shape
		Object crmcontracts.ImportObject `json:"object"`
	}, 0, len(rep.Kept))
	for _, k := range rep.Kept {
		kept = append(kept, struct {
			Id     openapi_types.UUID        `json:"id"` //nolint:staticcheck // matches the generated ImportUndoReport.Kept item shape
			Object crmcontracts.ImportObject `json:"object"`
		}{Id: openapi_types.UUID(k.ID), Object: crmcontracts.ImportObject(k.Object)})
	}
	errored := make([]struct {
		Id     openapi_types.UUID        `json:"id"` //nolint:staticcheck // matches the generated ImportUndoReport.Errored item shape
		Object crmcontracts.ImportObject `json:"object"`
		Reason string                    `json:"reason"`
	}, 0, len(rep.Errored))
	for _, e := range rep.Errored {
		errored = append(errored, struct {
			Id     openapi_types.UUID        `json:"id"` //nolint:staticcheck // matches the generated ImportUndoReport.Errored item shape
			Object crmcontracts.ImportObject `json:"object"`
			Reason string                    `json:"reason"`
		}{Id: openapi_types.UUID(e.ID), Object: crmcontracts.ImportObject(e.Object), Reason: e.Reason})
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

// withWarnings adds warnings that belong to one part rather than to a row.
func withWarnings(report migration.Report, object string, warnings []migration.SkippedRow) migration.Report {
	for i := range report.Objects {
		if report.Objects[i].Object == object {
			report.Objects[i].Warnings = append(report.Objects[i].Warnings, warnings...)
		}
	}
	return report
}

// withSkippedLines folds the source's own disclosures into the object's report.
// A row the source could not deliver never reached the writer, so nothing else
// in the report would mention it.
func withSkippedLines(report migration.Report, object string, skipped []migration.SkippedLine) migration.Report {
	if len(skipped) == 0 {
		return report
	}
	for i := range report.Objects {
		if report.Objects[i].Object != object {
			continue
		}
		for _, s := range skipped {
			report.Objects[i].Skipped = append(report.Objects[i].Skipped, migration.SkippedRow{
				ExternalID: fmt.Sprintf("line %d", s.Line),
				Line:       s.Line,
				Reason:     s.Reason,
			})
		}
	}
	return report
}
