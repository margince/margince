// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import "testing"

// An attempt that never reached the association phase does not get to report
// that the run linked nothing.
//
// Edges REPLACE rather than add, because the phase has no checkpoint and runs
// whole on every attempt — so adding would report a file's 12,000 employer
// links as 24,000. But a resume that failed before that phase answers zero
// applied and zero skipped, which is not an answer: taking it would erase what
// the earlier attempt actually linked.
func TestAnAttemptThatNeverLinkedDoesNotReportZeroLinks(t *testing.T) {
	earlier := Report{
		Associations:        12,
		AssociationsSkipped: []SkippedAssoc{{From: "a", To: "b", Reason: "no such company"}},
	}

	// A later attempt that died before the association phase.
	died := earlier.mergedWith(Report{})
	if died.Associations != 12 || len(died.AssociationsSkipped) != 1 {
		t.Errorf("associations = %d with %d skipped, want the earlier attempt's 12 and 1 — a "+
			"resume that never linked has not answered the question",
			died.Associations, len(died.AssociationsSkipped))
	}

	// A later attempt that DID walk them wins outright, which is the rule the
	// replacement exists for: it was computed against the estate as it stands.
	rewalked := earlier.mergedWith(Report{Associations: 9})
	if rewalked.Associations != 9 {
		t.Errorf("associations = %d, want the later attempt's 9", rewalked.Associations)
	}

	// So does one that walked them and found only refusals.
	refused := earlier.mergedWith(Report{
		AssociationsSkipped: []SkippedAssoc{{From: "c", To: "d", Reason: "two companies match"}},
	})
	if refused.Associations != 0 || len(refused.AssociationsSkipped) != 1 ||
		refused.AssociationsSkipped[0].From != "c" {
		t.Errorf("associations = %d with %+v, want the later attempt's own answer",
			refused.Associations, refused.AssociationsSkipped)
	}
}

// A report written before Attempts was carried reads as the one walk that wrote
// it, so an old report is not mistaken for a resumed one.
func TestAnUncountedReportIsOneWalk(t *testing.T) {
	for walks, want := range map[int]int{0: 1, 1: 1, 2: 2, 5: 5} {
		if got := (Report{Attempts: walks}).Walks(); got != want {
			t.Errorf("Report{Attempts: %d}.Walks() = %d, want %d", walks, got, want)
		}
	}
	// And folding two of those counts both.
	if got := (Report{}).mergedWith(Report{}).Walks(); got != 2 {
		t.Errorf("folding two uncounted reports gives %d walks, want 2", got)
	}
}

// A resume walks only the rows after its checkpoint, so the warnings of the
// rows before it must survive the fold.
func TestAResumedRunKeepsTheWarningsOfRowsBeforeTheCheckpoint(t *testing.T) {
	warned := func(ids ...string) Report {
		or := ObjectReport{Object: ObjectContact, File: "contacts.csv"}
		for _, id := range ids {
			or.Warnings = append(or.Warnings, SkippedRow{ExternalID: id, Kind: ProblemDroppedLink})
		}
		return Report{Objects: []ObjectReport{or}}
	}
	died := warned("P-1", "P-2")
	resumed := died.mergedWith(warned("P-7"))
	got := resumed.Objects[0]
	if len(got.Warnings) != 3 || got.Warnings[0].ExternalID != "P-1" || got.Warnings[2].ExternalID != "P-7" {
		t.Errorf("warnings = %+v, want P-1 and P-2 from before the checkpoint, then P-7", got.Warnings)
	}
	if got.File != "contacts.csv" {
		t.Errorf("file = %q, want contacts.csv kept", got.File)
	}
}

// A row's problem names its column, kind and value whether the row was skipped
// or landed with a warning, so a screen can group both.
func TestARowsProblemReachesItsSkipOrItsWarning(t *testing.T) {
	problem := &SkippedRow{Column: "company", Kind: ProblemDroppedLink, Value: "CO-404", Reason: "no company"}
	var or ObjectReport
	or.record(Row{ExternalID: "P-1", Line: 2}, EnsureResult{Created: true, Problem: problem})
	if or.Created != 1 || len(or.Skipped) != 0 || len(or.Warnings) != 1 {
		t.Fatalf("report = %+v, want one created row carrying one warning", or)
	}
	want := SkippedRow{ExternalID: "P-1", Line: 2, Column: "company", Kind: ProblemDroppedLink, Value: "CO-404", Reason: "no company"}
	if or.Warnings[0] != want {
		t.Fatalf("warning = %+v, want %+v", or.Warnings[0], want)
	}

	broken := &SkippedRow{Column: "company", Kind: ProblemBrokenLink, Value: "CO-404"}
	or.record(Row{ExternalID: "P-2", Line: 3}, EnsureResult{Skipped: true, SkipReason: "no company", Problem: broken})
	if len(or.Skipped) != 1 || or.Skipped[0].Kind != ProblemBrokenLink || or.Skipped[0].Value != "CO-404" ||
		or.Skipped[0].Column != "company" || or.Skipped[0].Reason != "no company" {
		t.Fatalf("skipped = %+v, want the skip to carry the problem's column, kind and value", or.Skipped)
	}
	if len(or.Warnings) != 1 {
		t.Fatalf("warnings = %d, want a skipped row's problem kept out of the warnings", len(or.Warnings))
	}
}
