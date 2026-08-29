// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading a run's report back onto the wire: what the import DID, in the file's
// own terms.
//
// Split from the request half of the wire because the two answer opposite
// questions — one takes a caller's mapping apart and refuses what the estate
// cannot receive, the other puts a finished run's outcome into a shape a person
// reads. They shared a file until it passed the length cap, and nothing but
// history held them together.

import (
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
	}

	// Predicted counts and actual ones are never summed: a finished run reports
	// what it did, an awaiting one what it will do. Adding them would report
	// twice the rows the file holds — and the stored report DOES carry both,
	// because a run's own report is merged into the dry run's so a resumed
	// attempt keeps what the earlier one already achieved.
	committed := run.Status == migration.StatusComplete || run.Status == migration.StatusFailed ||
		run.Status == migration.StatusUndoing || run.Status == migration.StatusUndone
	seen := map[string]bool{}
	duplicates := 0
	for _, o := range run.Report.Objects {
		out.RowsRead += o.MirrorCount
		if committed {
			out.Disposition.Created += o.Created
			out.Disposition.Updated += o.Updated
		} else {
			out.Disposition.Created += o.WillCreate
			out.Disposition.Updated += o.WillUpdate
		}
		// The commit's own count once there is one, for the same reason
		// Created replaces WillCreate above: the estate moves between the
		// preview and the approval — a colleague creates one of the companies,
		// an earlier run lands it — and a finished report that stated the
		// prediction would describe what was expected rather than what
		// happened.
		if committed {
			duplicates += o.Duplicated
		} else {
			duplicates += o.WillDuplicate
		}
		out.Disposition.Skipped += appendIssues(&out.Issues, seen, o.Skipped)
		// A collision is reported the same way a skip is — by line, in the
		// file's own terms — but it does NOT touch Disposition.Skipped: the
		// row is counted in Created, and adding it here too would report two
		// outcomes for one row and leave `unchanged` short by the difference.
		appendIssues(&out.Issues, seen, o.Collisions)
	}

	// Reported even when zero: "0 duplicates" is the answer to the question a
	// person is asking before they approve, and an omitted field reads as "not
	// checked" rather than "none found".
	out.Disposition.Duplicates = &duplicates
	out.Links = linksOf(run.Report, committed)

	// A finished run's stored report can carry more outcomes than the file has
	// rows, and there are two causes with different right answers.
	//
	// A row the DRY RUN skipped whose collision then vanished commits as a
	// CREATE, and the stale skip is stored beside the real create — the engine
	// folds attempts by object class and concatenates their skipped lists. Here
	// the skip is the entry to drop.
	//
	// A RESUMED run can also double-count: ObjectReport.record runs before
	// advanceCheckpoint, so a checkpoint that fails to persist leaves a counted
	// row the resume walks again, and attempt reports add their counts. Here the
	// surplus is in `created`/`updated`, and taking it out of `skipped` would
	// erase a genuine refusal.
	//
	// Nothing in the stored report distinguishes them: per-row entries exist for
	// skips alone, and everything else is an aggregate. Only the SECOND cause
	// needs a resume to happen at all, so the attempt count is what tells them
	// apart — a single-attempt run can only have the first.
	//
	// The OBJECT count cannot stand in for it. Attempts fold by object class, so
	// a run walked five times still reports one object — and a CSV import is
	// always exactly one class, so the guard was true on every CSV report and
	// took a resumed run's surplus out of `skipped`, erasing the refusals this
	// report exists to name.
	if committed && run.Report != nil {
		surplus := out.Disposition.Created + out.Disposition.Updated +
			out.Disposition.Skipped - out.RowsRead
		if surplus > 0 && run.Report.Walks() == 1 {
			out.Disposition.Skipped = max(out.Disposition.Skipped-surplus, 0)
		}
	}

	// Unchanged is DERIVED, never carried: it is the rows that were read and
	// then neither created, updated nor skipped. Carrying the stored figure
	// would add the dry run's count to the commit's and report more rows than
	// the file holds — which is the contract's own invariant ("the four sum to
	// the rows read") failing in the one place a human reads it.
	out.Disposition.Unchanged = max(
		out.RowsRead-out.Disposition.Created-out.Disposition.Updated-out.Disposition.Skipped,
		0,
	)
	// Present once the run has been undone, not while undoing — a
	// still-in-progress or interrupted reversal's partial counts are the
	// run's own internal resume state, not a finished outcome to report.
	if run.UndoReport != nil && run.Status == migration.StatusUndone {
		out.Undo = toContractUndoReport(run.ID, run.Status, *run.UndoReport)
	}
	return out
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
	issues *[]crmcontracts.ImportRowIssue, seen map[string]bool, rows []migration.SkippedRow,
) int {
	added := 0
	for _, row := range rows {
		key := row.ExternalID + "\x00" + strconv.Itoa(row.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		added++
		*issues = append(*issues, crmcontracts.ImportRowIssue{
			Line:   lineOf(row),
			Reason: row.Reason,
		})
	}
	return added
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
