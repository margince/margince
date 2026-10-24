// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const (
	commitmentStateOpen      = "open"
	commitmentPrecisionDay   = "day"
	commitmentPrecisionRange = "range"
	commitmentPrecisionNone  = "none"
	commitmentStateField     = "state"
	commitmentMeaningField   = "commitment"
)

var (
	commitmentMeaningStates      = []string{commitmentStateOpen, "completed", "conditional", "optional", "request", "unclear"}
	commitmentDeadlinePrecisions = []string{commitmentPrecisionDay, commitmentPrecisionRange, commitmentPrecisionNone}
)

var commitmentRangeOnly = regexp.MustCompile(`(?i)^(?:(?:by|during|sometime|in|bis|im|trong)\s+)?(?:(?:next|this|the coming)\s+(?:week|month|quarter|year)|nächste[nrsm]?\s+(?:woche|monat|quartal|jahr)|diese[nrsm]?\s+(?:woche|monat|quartal|jahr)|(?:tuần|tháng|quý|năm)\s+(?:tới|sau|này))$`)

type commitmentMeaning crmcontracts.CommitmentMeaning

func commitmentMeaningSchema() schema.Node {
	return schema.Object(map[string]schema.Node{
		commitmentStateField: schema.Enum(commitmentMeaningStates...),
		"condition":          schema.String(),
		"deadline_precision": schema.Enum(commitmentDeadlinePrecisions...),
		"deadline_quote":     schema.String(),
	}, commitmentStateField, "condition", "deadline_precision", "deadline_quote")
}

// A valid non-action is an answer; missing or contradictory meaning is a failed reading.
func validateCommitmentMeaning(meaning *commitmentMeaning, dueDate, source string) string {
	if meaning == nil || !slices.Contains(commitmentMeaningStates, meaning.State) {
		return "a commitment needs a known state before it can create work"
	}
	if !slices.Contains(commitmentDeadlinePrecisions, meaning.DeadlinePrecision) {
		return "a commitment needs the precision of its stated deadline"
	}
	if !validProposedDueDate(dueDate) || (meaning.DeadlinePrecision == commitmentPrecisionDay) != (dueDate != "") {
		return "only an explicitly stated day may supply an exact due date"
	}
	condition := strings.TrimSpace(meaning.Condition)
	if (meaning.State == "conditional") != (condition != "") {
		return "a conditional commitment must retain its unmet condition"
	}
	deadline := strings.TrimSpace(meaning.DeadlineQuote)
	if meaning.DeadlinePrecision == commitmentPrecisionDay && commitmentRangeOnly.MatchString(deadline) {
		return "a deadline range cannot supply one exact day"
	}
	if (meaning.DeadlinePrecision == commitmentPrecisionNone) != (deadline == "") {
		return "a stated deadline must retain its supporting words"
	}
	for _, quote := range []string{condition, deadline} {
		if quote == "" {
			continue
		}
		if utf8.RuneCountInString(quote) > maxExtractQuote {
			return "a commitment condition or deadline quote is too long"
		}
		if _, found := evidenceSpan(source, quote); !found {
			return "a commitment condition or deadline quotes words outside its evidence"
		}
	}
	return ""
}

func (meaning *commitmentMeaning) withheldOutcome() CommitmentOutcome {
	if meaning == nil || !slices.Contains(commitmentMeaningStates, meaning.State) {
		return "semantic_unassessed"
	}
	if meaning.State == commitmentStateOpen {
		return ""
	}
	return CommitmentOutcome("semantic_" + meaning.State)
}

const commitmentMeaningInstruction = `"commitment" describes whether work is still owed: ` +
	`state is "open" only for a definite, unfulfilled promise; "completed" for an act already done; ` +
	`"conditional" for a promise whose condition has not happened; "optional" for willingness or a tentative offer; ` +
	`"request" for an ask that has not been accepted as a promise; "unclear" when the evidence cannot decide. ` +
	`condition quotes the unmet condition exactly for conditional promises, and is empty otherwise. ` +
	`deadline_precision is "day" only when a single day is stated, "range" for wording such as next week, ` +
	`and "none" when no deadline is stated. deadline_quote copies the supporting deadline words exactly, ` +
	`or is empty for none. due_date must be empty for range and none; never invent a day from a range. ` +
	`A high confidence does not turn a completed act, request or conditional offer into an open promise. `

const commitmentOfferInstruction = `Classify willingness ("I could", "if helpful", "I will try") as optional, with an empty condition.
A conditional promise is definite work dependent on an external event, rather than an offer of help.
Keep a stated day even for optional work: its state prevents a task, not a missing due_date.`
