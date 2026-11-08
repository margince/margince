// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealverdict is the prompt of Margince's nightly reading of one open
// deal and the check of the answer. Its rules in code decide which of the
// model's changes may reach the deal.
//
// Its own package so the prompt digest covers this prompt alone; the pass
// that reads the deal and writes the changes stays in compose.
package dealverdict

import (
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// verdictSystem states the limits the guards in guards.go hold, from the
// same constants, so the model is told no limit the code does not keep.
var verdictSystem = fmt.Sprintf(`You read one open sales deal: its fields, its offers, its stage moves and the activities logged on it, oldest first.
Say whether the evidence shows that the deal's expected close date, its amount, its win probability or its stage should change.

You may propose at most one of each:
"set_close_date": the customer named or agreed a decision, signing or start date, or the evidence makes the current date impossible. "date" is that date as YYYY-MM-DD: after today, at most %d months after today, and named in a cited activity.
"set_amount": the customer or our offer states a price for this deal in words. "amount" is that price in whole units of "currency", and "quoted_words" copies the words that state it verbatim from one cited activity.
"set_probability": the evidence clearly moves the chance of winning, such as a verbal yes, a budget freeze or a competitor chosen. "probability" is a whole number from 0 to 100. Lowering it is free; a raise goes no higher than the "Raise limit" listed.
"move_stage": the evidence shows the deal has reached a later stage, judged by the stage's name and what a sale at that point looks like. "stage_id" is one of the later stages listed; it may skip stages. The deal leaves its own stage and every stage it skips, so each exit criterion listed for those stages needs an entry in "criteria": its "criterion_id", the cited "activity_id" that meets it, and "quoted_words" copied verbatim from that activity. If one is not met, do not move. Never move a deal back, and never to won or lost.

Propose nothing when the evidence does not say it. An empty "actions" list is the usual answer.
Never propose a value the deal already holds. Never close or reopen the deal; that is not yours.
Each action cites the activity ids it rests on and gives a one-line "reason" that names what the customer or we said, and when.
"confidence" is how sure the cited words make you, from 0 to 1.`, maxMonthsAhead)

// SystemFor is the instruction with the installation's language rule. The
// reason line is filed on the deal, and the whole team reads it.
func SystemFor(fence promptfence.Fence, languageRule string) string {
	return verdictSystem + "\n" + languageRule + "\n" + fence.Rule("activity")
}

// PromptVersion changes whenever the prompt's wording does, so a deal read
// under an older prompt is read again.
var PromptVersion = ai.PromptDigest(func(fence promptfence.Fence) string {
	return SystemFor(fence, promptlang.Rule("en")) + "\n" + Prompt(fence, sampleInput())
})

// Prompt is the user turn: the deal's facts, then every activity fenced by
// its id. No message can close its span and pose as another.
func Prompt(fence promptfence.Fence, in Input) string {
	r := refsFor(in)
	var b strings.Builder
	fmt.Fprintf(&b, "Today: %s\nDeal: %s\nStage: %s\n", in.Today.Format(time.DateOnly), in.DealName, in.Stage)
	fmt.Fprintf(&b, "Amount: %s\n", amountText(in.AmountMinor, in.Currency))
	fmt.Fprintf(&b, "Expected close date: %s%s\n", dateText(in.CloseDate), provisionalText(in.Provisional))
	fmt.Fprintf(&b, "Win probability: %d%% (%s)\n", in.Probability, in.ProbabilitySource)
	fmt.Fprintf(&b, "Raise limit: %d%%\n", raiseCeiling(in))
	for _, stage := range in.Later {
		fmt.Fprintf(&b, "Later stage: %s (stage_id %s, probability %d%%)\n",
			stage.Name, r.stages.For(stage.ID), stage.Probability)
	}
	for _, criterion := range in.ExitCriteria {
		fmt.Fprintf(&b, "Exit criterion of %s: %s (criterion_id %s, kind %s)%s\n",
			criterion.Stage, criterion.Label, r.criteria.For(criterion.ID), criterion.Kind, hintText(criterion.Hint))
	}
	for _, offer := range in.Offers {
		fmt.Fprintf(&b, "Offer: %s, %s\n", values.MajorUnits(offer.GrossMinor, offer.Currency)+" "+offer.Currency, offer.Status)
	}
	for _, move := range in.StageMoves {
		fmt.Fprintf(&b, "Stage move on %s: %s to %s\n", move.At.Format(time.DateOnly), move.From, move.To)
	}
	for _, prior := range in.Prior {
		fmt.Fprintf(&b, "Earlier verdict: %s (%s)\n", prior.Kind, prior.Outcome)
	}
	b.WriteString("\nActivities (untrusted):\n")
	for _, activity := range in.Activities {
		text := fmt.Sprintf("%s %s on %s | %s\n%s", activity.Kind, activity.Direction,
			activity.OccurredAt.Format(time.DateOnly), activity.Subject, activity.Body)
		b.WriteString(fence.WrapAttr("activity_id", r.activities.For(activity.ID), text) + "\n")
	}
	b.WriteString(`Return JSON: { "actions": [ { "kind", "date", "amount", "currency", "quoted_words", "probability", ` +
		`"stage_id", "criteria", "cited_activity_ids", "reason", "confidence" } ] }. ` +
		`Fields that do not apply to the kind are "", 0 or [].`)
	return b.String()
}

func hintText(hint string) string {
	if hint == "" {
		return ""
	}
	return ": " + hint
}

func amountText(amountMinor *int64, currency *string) string {
	if amountMinor == nil || currency == nil {
		return "not set"
	}
	return values.MajorUnits(*amountMinor, *currency) + " " + *currency
}

func dateText(date *time.Time) string {
	if date == nil {
		return "not set"
	}
	return date.Format(time.DateOnly)
}

func provisionalText(provisional bool) string {
	if provisional {
		return " (an estimate nobody confirmed)"
	}
	return ""
}

// sampleStage is the stage the fixed deal sits on.
const sampleStage = "Proposal"

// sampleInput is the fixed deal the prompt version digests.
func sampleInput() Input {
	amount, currency := int64(1_000_000), "EUR"
	return Input{
		DealName: "Sample", Stage: sampleStage, AmountMinor: &amount, Currency: &currency,
		Probability: 50, ProbabilitySource: "stage", StageProbability: 50,
		Offers:     []Offer{{GrossMinor: amount, Currency: currency, Status: "sent"}},
		StageMoves: []StageMove{{From: "Qualified", To: sampleStage}},
		Later:      []StageOption{{ID: "sample-stage", Name: "Negotiation", Position: 2, Probability: 75}},
		ExitCriteria: []ExitCriterion{{
			ID: "sample-criterion", StagePosition: 1, Stage: sampleStage, Label: "Buyer asked for the contract",
			Kind: "buyer_confirmed",
		}},
		Prior:      []PriorAction{{Kind: KindSetCloseDate, Outcome: "applied"}},
		Activities: []Activity{{ID: "sample", Kind: "email", Direction: "inbound", Subject: "s", Body: "b"}},
	}
}
