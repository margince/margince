// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

// The rules in code that decide which of the model's changes may reach a
// deal. The model proposes; nothing here trusts it further than the cited
// evidence goes.

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
)

// Why an action was not applied. The deal-row recheck adds its own
// (deals.VerdictDropped*).
const (
	OutcomeApply          = "apply"
	DroppedLowConfidence  = "dropped:low_confidence"
	DroppedDateOutOfRange = "dropped:date_out_of_range"
	DroppedHumanEdit      = "dropped:human_edit"
	DroppedHumanSet       = "dropped:human_probability"
	DroppedOfferFixes     = "dropped:accepted_offer"
	DroppedQuoteNotFound  = "dropped:quote_not_found"
	DroppedNoRate         = "dropped:no_rate"
	// DroppedOutOfRange is a converted amount no deal can hold. A new
	// rate is no reason to read the deal again, unlike DroppedNoRate.
	DroppedOutOfRange     = "dropped:amount_out_of_range"
	DroppedSwitchOff      = "dropped:switch_off"
	DroppedUndoHold       = "dropped:undo_hold"
	DroppedUnchanged      = "dropped:unchanged"
	DroppedFutureCitation = "dropped:future_citation"
	DroppedAmountMismatch = "dropped:amount_not_quoted"
	DroppedDateNotStated  = "dropped:date_not_in_evidence"
	DroppedAboveStage     = "dropped:above_stage"
)

// ConfidenceFloor is the least confidence an action needs. Below it nothing
// is changed and nothing is asked.
const ConfidenceFloor = 0.7

// maxMonthsAhead bounds a close date Margince sets.
const maxMonthsAhead = 18

// Decision is one proposed action and what the rules made of it. Change is
// set only when the outcome is OutcomeApply.
type Decision struct {
	Action  Action
	Outcome string
	Change  *deals.VerdictChange
}

// Rates answers what a currency converts to the base currency at.
type Rates interface {
	For(currency string) (deals.FXRate, bool, error)
}

// Decide runs every rule over the model's answer, in the answer's order.
func Decide(in Input, state State, payload Payload, rates Rates) ([]Decision, error) {
	out := make([]Decision, 0, len(payload.Actions))
	for _, action := range payload.Actions {
		decision, err := decideOne(in, state, action, rates)
		if err != nil {
			return nil, err
		}
		out = append(out, decision)
	}
	return out, nil
}

func decideOne(in Input, state State, action Action, rates Rates) (Decision, error) {
	correction := correctionOf(action.Kind)
	newest := newestCited(in, action.Cited)
	change := deals.VerdictChange{
		Correction: correction, NewestCitedAt: newest,
		Evidence: map[string]any{"basis": action.Reason, "cited_activity_ids": action.Cited},
	}
	drop := func(why string) (Decision, error) { return Decision{Action: action, Outcome: why}, nil }
	switch {
	case float64(action.Confidence) < ConfidenceFloor:
		return drop(DroppedLowConfidence)
	case citesTheFuture(in, action.Cited):
		return drop(DroppedFutureCitation)
	case !state.SwitchedOn[correction]:
		return drop(DroppedSwitchOff)
	case state.Held[correction]:
		return drop(DroppedUndoHold)
	case editedAfter(state.HumanEditedAt, correction, newest):
		return drop(DroppedHumanEdit)
	}
	var why string
	var err error
	switch action.Kind {
	case KindSetCloseDate:
		why = decideCloseDate(in, state, action, &change)
	case KindSetProbability:
		why = decideProbability(in, state, action, &change)
	case KindSetAmount:
		why, err = decideAmount(in, state, action, rates, &change)
	case KindMoveStage:
		why = decideMoveStage(in, action, &change)
	}
	if err != nil || why != "" {
		return Decision{Action: action, Outcome: why}, err
	}
	return Decision{Action: action, Outcome: OutcomeApply, Change: &change}, nil
}

func decideCloseDate(in Input, state State, action Action, change *deals.VerdictChange) string {
	date, err := time.Parse(time.DateOnly, action.Date)
	if err != nil || !date.After(in.Today) || date.After(in.Today.AddDate(0, maxMonthsAhead, 0)) {
		return DroppedDateOutOfRange
	}
	if !dateInEvidence(in, action.Cited, date) {
		return DroppedDateNotStated
	}
	if state.Seen.CloseDate != nil && state.Seen.CloseDate.Format(time.DateOnly) == action.Date &&
		!state.Seen.Provisional {
		return DroppedUnchanged
	}
	change.CloseDate = date
	return ""
}

func decideProbability(in Input, state State, action Action, change *deals.VerdictChange) string {
	seen := state.Seen
	if seen.ProbabilitySource != nil && deals.Source(*seen.ProbabilitySource) == deals.SourceHuman {
		return DroppedHumanSet
	}
	if in.Probability == action.Probability {
		return DroppedUnchanged
	}
	// Lowering is free. A raise stops at the stage's own probability, so no
	// mail can talk a deal above what its stage says.
	if action.Probability > raiseCeiling(in) {
		return DroppedAboveStage
	}
	change.Probability = action.Probability
	return ""
}

// raiseCeiling is the highest probability a raise may reach: the stage's own,
// or the deal's current one where that is already higher.
func raiseCeiling(in Input) int {
	return max(in.Probability, in.StageProbability)
}

// editedAfter reports whether a colleague wrote the correction's field after
// the newest evidence the action cites.
func editedAfter(edited map[string]time.Time, correction string, newest time.Time) bool {
	at, ok := edited[correction]
	return ok && at.After(newest)
}

// newestCited is when the newest cited activity arrived on the server.
func newestCited(in Input, cited []string) time.Time {
	known := in.activityByID()
	var newest time.Time
	for _, id := range cited {
		if activity, ok := known[id]; ok && activity.ArrivedAt.After(newest) {
			newest = activity.ArrivedAt
		}
	}
	return newest
}

// citesTheFuture reports whether a cited activity is dated after the read.
func citesTheFuture(in Input, cited []string) bool {
	known := in.activityByID()
	for _, id := range cited {
		if activity, ok := known[id]; ok && activity.OccurredAt.After(in.ReadAt) {
			return true
		}
	}
	return false
}

// quoteFound reports whether the quoted words appear in one cited activity,
// ignoring case and spacing.
func quoteFound(in Input, cited []string, quote string) bool {
	want := normalize(quote)
	if want == "" {
		return false
	}
	known := in.activityByID()
	for _, id := range cited {
		activity, ok := known[id]
		if ok && strings.Contains(normalize(activity.Subject+"\n"+activity.Body), want) {
			return true
		}
	}
	return false
}

func normalize(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func correctionOf(kind string) string {
	switch kind {
	case KindSetCloseDate:
		return deals.VerdictCloseDateCorrection
	case KindSetAmount:
		return deals.VerdictAmountCorrection
	case KindMoveStage:
		return deals.VerdictStageMoveCorrection
	default:
		return deals.VerdictProbabilityCorrection
	}
}
