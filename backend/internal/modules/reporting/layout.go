// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"regexp"
	"slices"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// cardID is the short id a client mints for a card, the contract's pattern.
var cardID = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// deriveLayoutBlocks sets Blocks from Layout: each kind once, in the order the
// layout first names it. The evaluation still reads Blocks, and so does a
// client that predates layouts.
func deriveLayoutBlocks(selection *crmcontracts.ReportingSelection) {
	if selection.Layout == nil {
		return
	}
	blocks := []crmcontracts.ReportingBlockKind{}
	for _, section := range *selection.Layout {
		for _, card := range section.Cards {
			kind := crmcontracts.ReportingBlockKind(card.Kind)
			if card.Kind != questionKind && !slices.Contains(blocks, kind) {
				blocks = append(blocks, kind)
			}
		}
	}
	selection.Blocks = blocks
}

// Kinds the evaluation draws as one chart for the whole report. It merges the
// two SDR series. Owner attainment shows bookings whenever bookings is
// selected. A card cannot ask for one metric's chart of these.
var oneChartKinds = []crmcontracts.ReportingBlockKind{reportingOwnerAttainment, "sdr_outcomes"}

// metric_reading is the metric details table, which the page always draws
// below the charts. It is never a chart, so no card can place it.
const unplacedKind crmcontracts.ReportingBlockKind = "metric_reading"

// questionKind is a card that asks a custom analytics question. It is not a
// chart of the evaluation, so it adds no block.
const questionKind crmcontracts.ReportingCardKind = "question"

// cardTarget is what one card draws. Two cards with one target draw the same
// chart twice, which no reader asked for.
type cardTarget struct {
	kind   crmcontracts.ReportingBlockKind
	metric crmcontracts.ReportingMetricID
}

// validateLayout checks every card against the metrics the report selects:
// metricBlocks maps each selected metric to the chart kinds it has.
func validateLayout(
	layout []crmcontracts.ReportingSection,
	metricBlocks map[crmcontracts.ReportingMetricID][]crmcontracts.ReportingBlockKind,
) error {
	if len(layout) > 8 {
		return refuse("selection.layout", "out_of_range", "use at most eight sections")
	}
	ids := map[string]bool{}
	targets := map[cardTarget]bool{}
	cards := 0
	for _, section := range layout {
		if section.Title != nil && utf8.RuneCountInString(*section.Title) > 120 {
			return refuse("selection.layout", "invalid", "title a section in at most 120 characters")
		}
		for _, card := range section.Cards {
			cards++
			if cards > 20 {
				return refuse("selection.layout", "out_of_range", "place at most twenty cards")
			}
			if err := validateCard(card, metricBlocks, ids, targets); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateCard checks one card's id and width, then what it draws. ids and
// targets are the cards already placed, which this one may not repeat.
func validateCard(
	card crmcontracts.ReportingCard,
	metricBlocks map[crmcontracts.ReportingMetricID][]crmcontracts.ReportingBlockKind,
	ids map[string]bool, targets map[cardTarget]bool,
) error {
	if !cardID.MatchString(card.Id) || ids[card.Id] {
		return refuse("selection.layout", "invalid", "give each card its own short id")
	}
	ids[card.Id] = true
	if card.Width != crmcontracts.ReportingCardWidthFull && card.Width != crmcontracts.ReportingCardWidthHalf {
		return refuse("selection.layout", "invalid", "make a card full or half width")
	}
	if card.Kind == questionKind || card.Question != nil {
		return validateQuestionCard(card)
	}
	target, err := cardTargetOf(card, metricBlocks)
	if err != nil {
		return err
	}
	if overlaps(targets, target) {
		return refuse("selection.layout", "duplicate", "place each chart once")
	}
	targets[target] = true
	return nil
}

// cardTargetOf checks that a card names a chart some selected metric has. A
// card that names a metric needs that metric selected and holding the chart.
func cardTargetOf(
	card crmcontracts.ReportingCard,
	metricBlocks map[crmcontracts.ReportingMetricID][]crmcontracts.ReportingBlockKind,
) (cardTarget, error) {
	kind := crmcontracts.ReportingBlockKind(card.Kind)
	if kind == unplacedKind {
		return cardTarget{}, refuse("selection.layout", "invalid", "the metric details table is not a card")
	}
	if card.Metric != nil && slices.Contains(oneChartKinds, kind) {
		return cardTarget{}, refuse("selection.layout", "invalid", "this chart covers every selected metric, so place it without one")
	}
	if card.Metric != nil {
		if !slices.Contains(metricBlocks[*card.Metric], kind) {
			return cardTarget{}, refuse("selection.layout", "invalid", "place a chart of a selected metric that has it")
		}
		return cardTarget{kind: kind, metric: *card.Metric}, nil
	}
	for _, blocks := range metricBlocks {
		if slices.Contains(blocks, kind) {
			return cardTarget{kind: kind}, nil
		}
	}
	return cardTarget{}, refuse("selection.layout", "invalid", "place a chart of a selected metric that has it")
}

// overlaps says whether a placed card already draws this target's chart. A card
// with no metric draws its kind for every metric, so it overlaps any card of
// that kind.
func overlaps(placed map[cardTarget]bool, target cardTarget) bool {
	if placed[target] || placed[cardTarget{kind: target.kind}] {
		return true
	}
	if target.metric != "" {
		return false
	}
	for other := range placed {
		if other.kind == target.kind {
			return true
		}
	}
	return false
}

// maxQuestionRows bounds the rows a question card shows, below the engine's own
// cap so the card's "first N rows" line is the true count.
const maxQuestionRows = 1000

// validateQuestionCard checks a card's analytics question. Its shape leaves out
// filters, scope and save: every reader receives the question, and it is asked
// afresh for each of them. A bound card's filters stay on the server. Whether
// its fields exist is the analytics engine's to answer when asked.
func validateQuestionCard(card crmcontracts.ReportingCard) error {
	question := card.Question
	switch {
	case card.Kind != questionKind || question == nil:
		return refuse("selection.layout", "invalid", "a question card carries its question, and only a question card does")
	case card.Metric != nil:
		return refuse("selection.layout", "invalid", "a question card names no metric")
	case question.SavedQuestionId != nil && question.Binding != nil:
		return refuse("selection.layout", "invalid", "bind a card to a new saved question, or keep the one it has, not both")
	case !values.HasVisibleText(question.Entity) || len(question.Measures) == 0 || len(question.Measures) > 8:
		return refuse("selection.layout", "invalid", "a question names its population and one to eight measures")
	case question.GroupBy != nil && len(*question.GroupBy) > 4:
		return refuse("selection.layout", "out_of_range", "group a question card by at most four fields")
	case question.Limit != nil && (*question.Limit < 1 || *question.Limit > maxQuestionRows):
		return refuse("selection.layout", "out_of_range", "show between one and a thousand rows on a question card")
	}
	return nil
}
