// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

type searchFilterOperand struct {
	Field  string          `json:"field"`
	Op     string          `json:"op"`
	Value  json.RawMessage `json:"value,omitempty"`
	Values json.RawMessage `json:"values,omitempty"`
}

var (
	countQuestion   = regexp.MustCompile(`(?i)\b(?:how many|count|wie viele)\b`)
	wonQuestion     = regexp.MustCompile(`(?i)\b(?:won|win|wins|gewonnen|gewinnen|gewinn)\b`)
	searchYearRange = regexp.MustCompile(`(?i)\b(?:from|von|từ)\s+(\d{4})\s+(?:to|bis|đến)\s+(\d{4})\b`)
)

// Admit rejects incomplete interpretations before any reader executes them.
func Admit(plan Plan, question, vocabulary string) (Plan, error) {
	if plan.Engine == EngineUnsupported || len(plan.UnsupportedParts) > 0 {
		return Plan{Engine: EngineUnsupported, Arguments: "{}"}, nil
	}
	if err := admitWonCount(plan, question, vocabulary); err != nil {
		return plan, err
	}
	if plan.Engine == EngineRecords || plan.Engine == EngineAnalytics {
		var metadata struct {
			Context crmcontracts.AnalyticsContext `json:"context"`
		}
		if err := json.Unmarshal([]byte(vocabulary), &metadata); err != nil {
			return plan, err
		}
		raw, err := searchMoneyOperands(json.RawMessage(plan.Arguments), metadata.Context.BaseCurrency)
		if err != nil {
			return plan, err
		}
		plan.Arguments = string(raw)
	}
	return plan, nil
}

func admitWonCount(plan Plan, question, vocabulary string) error {
	// Both, in either order: Vietnamese puts the verb first ("thắng bao nhiêu deal").
	words := vietnameseWords(question)
	asksCount := countQuestion.MatchString(question) || asksVietnameseCount(words)
	asksWon := wonQuestion.MatchString(question) || saysVietnameseWon(words)
	if !asksCount || !asksWon {
		return nil
	}
	if plan.Engine != EngineReporting || !plan.Count {
		return errors.New("won-deal counts must use the Sales won metric")
	}
	var selection crmcontracts.ReportingSelection
	if err := DecodeArguments(plan.Arguments, &selection); err != nil {
		return err
	}
	if len(selection.Metrics) != 1 || selection.Metrics[0] != crmcontracts.ReportingMetricIDBookingsWon {
		return errors.New("won-deal counts use bookings_won")
	}
	years := searchYearRange.FindStringSubmatch(question)
	if len(years) != 3 {
		return nil
	}
	first, err := strconv.Atoi(years[1])
	if err != nil {
		return err
	}
	last, err := strconv.Atoi(years[2])
	if err != nil {
		return err
	}
	if selection.Period != "custom" || selection.Interval == nil || !searchWholeYears(*selection.Interval, first, last, vocabulary) {
		return errors.New("the custom interval must include both complete named years")
	}
	return nil
}

func searchWholeYears(interval crmcontracts.ReportingWindow, first, last int, vocabulary string) bool {
	var metadata struct {
		Context crmcontracts.AnalyticsContext `json:"context"`
	}
	if err := json.Unmarshal([]byte(vocabulary), &metadata); err != nil {
		return false
	}
	zone, err := time.LoadLocation(metadata.Context.Timezone)
	if err != nil {
		return false
	}
	return first <= last && interval.StartAt.Equal(time.Date(first, 1, 1, 0, 0, 0, 0, zone)) && interval.EndAt.Equal(time.Date(last+1, 1, 1, 0, 0, 0, 0, zone))
}

// Money operands arrive in major units, as in filter_propose; the readers only receive MinorUnits results.
func searchMoneyOperands(raw json.RawMessage, baseCurrency string) (json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	for _, key := range []string{"plan", "traverse"} {
		if nested, ok := object[key]; ok && string(nested) != jsonNull {
			normal, err := searchMoneyOperands(nested, baseCurrency)
			if err != nil {
				return nil, err
			}
			object[key] = normal
		}
	}
	for _, key := range []string{"where", "filters"} {
		held, ok := object[key]
		if !ok {
			continue
		}
		encoded, err := searchMoneyFilters(held, baseCurrency)
		if err != nil {
			return nil, err
		}
		object[key] = encoded
	}
	return json.Marshal(object)
}

func searchMoneyFilters(held json.RawMessage, baseCurrency string) (json.RawMessage, error) {
	var filters []searchFilterOperand
	if err := DecodeArguments(string(held), &filters); err != nil {
		return nil, err
	}
	currency, err := filterCurrency(filters)
	if err != nil {
		return nil, err
	}
	for index, filter := range filters {
		if filter.Op == "exists" || !strings.HasSuffix(filter.Field, "_minor") && !strings.HasSuffix(filter.Field, "_minor_base") {
			continue
		}
		unit := currency
		if strings.HasSuffix(filter.Field, "_minor_base") {
			unit = baseCurrency
		}
		if unit == "" {
			return nil, errors.New("a money filter requires one explicit currency")
		}
		operand := &filters[index].Value
		if len(filter.Values) > 0 {
			operand = &filters[index].Values
		}
		minor, err := searchMinorOperand(*operand, unit)
		if err != nil {
			return nil, err
		}
		*operand = minor
	}
	return json.Marshal(filters)
}

func filterCurrency(filters []searchFilterOperand) (string, error) {
	currency := ""
	for _, filter := range filters {
		if filter.Field == "currency" && filter.Op == "eq" {
			if err := json.Unmarshal(filter.Value, &currency); err != nil {
				return "", err
			}
		}
	}
	return currency, nil
}

func searchMinorOperand(raw json.RawMessage, currency string) (json.RawMessage, error) {
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		var list []json.RawMessage
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
		for index, value := range list {
			minor, err := searchMinorOperand(value, currency)
			if err != nil {
				return nil, err
			}
			list[index] = minor
		}
		return json.Marshal(list)
	}
	minor, ok := values.MinorUnits(string(raw), currency)
	if !ok {
		return nil, errors.New("money operands must be exact major-unit numbers")
	}
	return json.Marshal(minor)
}

// SupportsMetric refuses values or counts the shared reporting reader withheld.
func SupportsMetric(evaluation crmcontracts.ReportingEvaluation, count bool) bool {
	if len(evaluation.Metrics) != 1 {
		return false
	}
	metric := evaluation.Metrics[0]
	if count {
		return metric.Coverage.EligibleCount != nil
	}
	return !metric.Coverage.Withheld && metric.Value != nil
}
