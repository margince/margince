// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package goalproposal proposes one seller's twelve monthly sales goals from
// their closed revenue. It builds the request, reads the reply and spreads a
// yearly goal over the proposed pattern.
//
// It writes nothing. The manager reviews the months in the editor and saves
// them as a draft, which belongs to modules/reporting.
package goalproposal

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/salesprogress"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// PromptVersion changes when the prompt or the reply shape changes.
const PromptVersion = "2026-11-12.1"

// The reply's bounds. A month above the ceiling is a misread, not a goal.
const (
	monthsInYear   = 12
	maxReasonRunes = 300
	maxMonthMajor  = int64(1_000_000_000_000)
)

// YearIn is one calendar year's closed revenue by month, in whole major
// units. A nil month has not ended or could not be read.
type YearIn struct {
	Year   int      `json:"year"`
	Months []*int64 `json:"months"`
}

// Input is what the model reads: the plan year and the closed revenue of
// that year and the year before. A yearly goal is in it when the reader set one.
type Input struct {
	Year       int      `json:"year"`
	Currency   string   `json:"currency"`
	Today      string   `json:"today"`
	YearlyGoal *int64   `json:"yearly_goal,omitempty"`
	Closed     []YearIn `json:"closed_revenue"`
	// YearlyGoalMinor is the yearly goal to the minor unit, which the months
	// add up to. The model reads it in whole major units.
	YearlyGoalMinor int64  `json:"-"`
	Lang            string `json:"-"`
}

// HasHistory reports whether any month of the input closed revenue, which is
// the least a pattern can be read from.
func (in Input) HasHistory() bool {
	for _, year := range in.Closed {
		for _, month := range year.Months {
			if month != nil && *month > 0 {
				return true
			}
		}
	}
	return false
}

const system = `You propose twelve monthly sales goals for one seller for the calendar year "year", from a JSON of their closed revenue by month.
Return ONLY a JSON object with these keys:
{"months":[<twelve whole numbers, January to December>],"reason":"..."}
"closed_revenue" holds the plan year and the year before, each with twelve months in whole units of "currency". A null month has not ended or could not be read. It is never zero.
Follow the seasonal pattern the closed revenue shows: a month that closed more gets a larger goal, and a month that closed little gets a smaller one, never zero because of it. Where the closed revenue shows no pattern, spread evenly.
When "yearly_goal" is given, the twelve months spread it and add up to it. Without it, choose the yearly level the closed revenue supports: the level of the last full year, raised only when the ended months of the plan year run above the same months of the year before.
Each month is a whole number of zero or more, with no separator and no unit. A plan holds all twelve months, the ended ones too.
"reason" is one or two plain sentences, at most 300 characters, that state the pattern the months follow and where it comes from, such as a stronger autumn after a quiet summer. It is saved as the reason of the plan and the seller reads it, so state it as a fact about the plan, with no "I" and no "you". Write no digit and no amount in it.
`

// Request is the one request production sends for this site.
//
//promptvoice:exempt the only prose is the plan's reason, saved under the manager's name, so it states the pattern and does not speak as the product.
func Request(in Input) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         system + "\n" + promptlang.Rule(in.Lang) + "\n" + fence.Rule("closed revenue"),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(encode(in))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		SecretStripper: ai.NewSecretStripper(),
	}
}

func encode(in Input) string {
	encoded, err := json.Marshal(in)
	if err != nil {
		return fmt.Sprintf("{%q:%q}", "encoding_error", err.Error())
	}
	return string(encoded)
}

// Proposal is the twelve months in minor units of the plan currency, and the
// pattern they follow in words.
type Proposal struct {
	Months []int64
	Reason string
}

// Total is what the twelve months add up to.
func (p Proposal) Total() int64 {
	var total int64
	for _, month := range p.Months {
		total += month
	}
	return total
}

type replyShape struct {
	Months []json.Number `json:"months"`
	Reason string        `json:"reason"`
}

// Parse reads the reply into a proposal. It wants twelve whole months of zero
// or more, with at least one above zero. It refuses a reason that is blank,
// too long or carries a digit. A yearly goal is spread over the months
// as weights, so the proposal adds up to it to the minor unit.
func Parse(reply string, in Input) (Proposal, error) {
	var parsed replyShape
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return Proposal{}, fmt.Errorf("parse goal proposal: %w", err)
	}
	if len(parsed.Months) != monthsInYear {
		return Proposal{}, fmt.Errorf("the proposal holds %d months, not twelve", len(parsed.Months))
	}
	major := make([]int64, monthsInYear)
	var sum int64
	for index, month := range parsed.Months {
		amount, err := month.Int64()
		if err != nil || amount < 0 || amount > maxMonthMajor {
			return Proposal{}, fmt.Errorf("month %d of the proposal is not a whole amount of zero or more", index+1)
		}
		major[index] = amount
		sum += amount
	}
	if sum == 0 {
		return Proposal{}, errors.New("the proposal sets no month above zero")
	}
	reason, err := keepReason(parsed.Reason)
	if err != nil {
		return Proposal{}, err
	}
	if in.YearlyGoal != nil {
		return Proposal{Months: salesprogress.SettleShares(in.YearlyGoalMinor, major), Reason: reason}, nil
	}
	scale := values.MinorUnitScale(in.Currency)
	for index := range major {
		major[index] *= scale
	}
	return Proposal{Months: major, Reason: reason}, nil
}

func keepReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return "", errors.New("the proposal gives no reason")
	case utf8.RuneCountInString(reason) > maxReasonRunes:
		return "", errors.New("the proposal's reason is too long")
	case strings.ContainsFunc(reason, unicode.IsDigit):
		return "", errors.New("the proposal's reason carries a figure")
	}
	return reason, nil
}
