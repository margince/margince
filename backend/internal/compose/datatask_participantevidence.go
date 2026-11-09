// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

func participantEvidenceDataTask() datatask.Task {
	step := foldStep("project_participant_evidence", "the participant insert trigger stores the same derived parent clock without a separate audit",
		activities.FoldParticipantEvidenceTx, activities.CountParticipantEvidence)
	step.EmptyWhenDone = true
	return datatask.Task{
		ID: "core/1793984655_participant_domain_evidence", Version: 1,
		Title: "Index participant domain evidence", Requires: []string{"core/1793984654"},
		Steps: []datatask.Step{step},
	}
}

func participantEvidenceRepairLimit(progress []datatask.Progress) int {
	task := participantEvidenceDataTask()
	limit := task.Steps[0].Limit
	for _, step := range task.Steps {
		complete := false
		for _, p := range progress {
			if p.TaskID == task.ID && p.Step == step.Name && p.TaskVersion == task.Version && p.Status == datatask.Done {
				complete = true
				limit = min(limit, max(step.MinLimit, min(step.Limit, p.BatchLimit)))
			}
		}
		if !complete {
			return 0
		}
	}
	return limit
}
