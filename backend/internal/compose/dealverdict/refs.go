// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// refs names the records one request shows: activities a1…, later stages s1…
// and exit criteria c1…, each in the order the prompt lists them.
type refs struct {
	activities, stages, criteria *promptref.Refs
}

func refsFor(in Input) refs {
	r := refs{activities: promptref.New("a"), stages: promptref.New("s"), criteria: promptref.New("c")}
	for _, activity := range in.Activities {
		r.activities.For(activity.ID)
	}
	for _, stage := range in.Later {
		r.stages.For(stage.ID)
	}
	for _, criterion := range in.ExitCriteria {
		r.criteria.For(criterion.ID)
	}
	return r
}

// readRefs rewrites each ref the answer names to the record id it stands for,
// and names the first ref this request never showed. "" stays "": it is the
// stage and criterion of an action that is not a move.
func readRefs(payload *Payload, r refs) string {
	for i := range payload.Actions {
		action := &payload.Actions[i]
		for j, ref := range action.Cited {
			id, ok := r.activities.ID(ref)
			if !ok {
				return fmt.Sprintf("cited id %q is not one of the supplied activities", ref)
			}
			action.Cited[j] = id
		}
		if action.StageID != "" {
			id, ok := r.stages.ID(action.StageID)
			if !ok {
				return fmt.Sprintf("stage_id %q is not one of the later stages listed", action.StageID)
			}
			action.StageID = id
		}
		for j := range action.Criteria {
			if msg := readCiteRefs(&action.Criteria[j], r); msg != "" {
				return msg
			}
		}
	}
	return ""
}

func readCiteRefs(cite *CriterionCite, r refs) string {
	criterion, ok := r.criteria.ID(cite.CriterionID)
	if !ok {
		return fmt.Sprintf("criterion_id %q is not one of the exit criteria listed", cite.CriterionID)
	}
	activity, ok := r.activities.ID(cite.ActivityID)
	if !ok {
		return fmt.Sprintf("criterion %q rests on activity %q, which is not one of the supplied activities",
			cite.CriterionID, cite.ActivityID)
	}
	cite.CriterionID, cite.ActivityID = criterion, activity
	return ""
}
