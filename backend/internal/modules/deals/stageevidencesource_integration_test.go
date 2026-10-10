// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// Stage evidence outlives the record it was read from. The source pair has no
// foreign key, so a hard delete of a cited activity goes through. Every reader
// of the ledger then reports the source as gone instead of failing on it.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEvidenceStillReadsAfterItsActivityIsDeleted(t *testing.T) {
	e := setupConfigEnv(t)
	dealID, fromID, _ := twoStagePipeline(t, e)
	criterion := addCriterion(t, e, fromID, "problem_confirmed")
	criterionID := ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id))
	meetingID := seedTranscribedMeeting(t, e, dealID, "")

	cited := claim(dealID, criterionID, AuthorBuyer)
	cited.SourceID = meetingID
	if _, err := e.store.RecordStageEvidence(e.asDealWriter(), cited); err != nil {
		t.Fatalf("recording evidence read from the meeting: %v", err)
	}
	before, err := e.store.ListStageEvidence(e.asDealWriter(), dealID)
	if err != nil {
		t.Fatalf("listing before the delete: %v", err)
	}
	if len(before) != 1 || before[0].SourceMissing {
		t.Fatalf("precondition: one row whose source exists, got %+v", before)
	}
	if got := facts(t, e, dealID); len(got.Criteria[0].Sources) != 1 || got.Criteria[0].Sources[0].Gone {
		t.Fatalf("precondition: the criterion cites a source that exists, got %+v", got.Criteria[0].Sources)
	}

	// No product path hard-deletes an activity, so the delete is SQL.
	if _, err := e.owner.Exec(t.Context(), `DELETE FROM activity WHERE id = $1`, meetingID); err != nil {
		t.Fatalf("hard-deleting the cited meeting: %v", err)
	}

	after, err := e.store.ListStageEvidence(e.asDealWriter(), dealID)
	if err != nil {
		t.Fatalf("the ledger stopped reading once its source was deleted: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("the ledger holds %d rows after the delete, want the one observation kept", len(after))
	}
	if !after[0].SourceMissing {
		t.Error("the deleted meeting reads as present, so a client follows source_id to nothing")
	}
	if ids.UUID(after[0].SourceId) != meetingID {
		t.Errorf("source_id = %v, want the deleted meeting's id %v kept", after[0].SourceId, meetingID)
	}

	got := facts(t, e, dealID)
	if !got.Criteria[0].Met {
		t.Error("the criterion lost its settlement when its source was deleted; the observation outlives it")
	}
	if len(got.Criteria[0].Sources) != 1 || !got.Criteria[0].Sources[0].Gone {
		t.Errorf("the progression facts cite %+v, want the one source marked gone", got.Criteria[0].Sources)
	}
}
