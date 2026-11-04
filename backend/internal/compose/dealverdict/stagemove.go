// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Why a stage move was not applied.
const (
	DroppedNotForward     = "dropped:stage_not_forward"
	DroppedCriterionUnmet = "dropped:criterion_unmet"
)

// decideMoveStage lets a move through only to a later open stage, and only
// when every required exit criterion of each stage the deal leaves rests on a
// cited activity that holds the quoted words and fits the criterion's kind.
func decideMoveStage(in Input, action Action, change *deals.VerdictChange) string {
	target, ok := laterStage(in, action.StageID)
	if !ok {
		return DroppedNotForward
	}
	toStage, err := ids.ParseAs[ids.StageKind](target.ID)
	if err != nil {
		return DroppedNotForward
	}
	met := make([]ids.UUID, 0, len(in.ExitCriteria))
	for _, criterion := range in.ExitCriteria {
		if criterion.StagePosition >= target.Position {
			continue
		}
		cite, ok := citeFor(action, criterion.ID)
		if !ok || !criterionMet(in, criterion, cite) {
			return DroppedCriterionUnmet
		}
		id, err := ids.Parse(criterion.ID)
		if err != nil {
			return DroppedCriterionUnmet
		}
		met = append(met, id)
	}
	change.ToStageID = toStage
	change.CriteriaMet = met
	change.Evidence["to_stage"] = target.Name
	return ""
}

func laterStage(in Input, id string) (StageOption, bool) {
	for _, stage := range in.Later {
		if stage.ID == id && stage.Position > in.StagePosition {
			return stage, true
		}
	}
	return StageOption{}, false
}

func citeFor(action Action, criterionID string) (CriterionCite, bool) {
	for _, cite := range action.Criteria {
		if cite.CriterionID == criterionID {
			return cite, true
		}
	}
	return CriterionCite{}, false
}

// criterionMet checks one criterion's citation by rules the model cannot
// talk past. A buyer milestone needs the buyer's own mail. A held event needs
// a meeting or a call. A legal review is settled only by an approved review,
// never by evidence Margince reads.
func criterionMet(in Input, criterion ExitCriterion, cite CriterionCite) bool {
	activity, ok := in.activityByID()[cite.ActivityID]
	if !ok || !quoteFound(in, []string{cite.ActivityID}, cite.QuotedWords) {
		return false
	}
	kind := deals.CriterionKind(criterion.Kind)
	switch {
	case kind == deals.CriterionLegalReviewed:
		return false
	case kind == deals.CriterionEventHeld:
		return activity.Kind == "meeting" || activity.Kind == "call"
	case kind.BuyerMilestone():
		return activity.Kind == "email" && activity.Direction == "inbound"
	}
	return true
}
