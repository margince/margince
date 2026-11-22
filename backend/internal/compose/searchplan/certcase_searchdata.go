// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Cases builds certification prompts from the production reader vocabulary.
type Cases struct{ Vocabulary func(Fixture) (string, error) }

// Site binds certification to the registered planner site.
func (Cases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskNlSearch, Variant: "search_data_plan", Kind: ai.SiteKindOneShot}
}

// Fixture holds a question and its permitted reader catalogs.
type Fixture struct {
	Question   string                        `json:"question"`
	Vocabulary string                        `json:"vocabulary"`
	Now        time.Time                     `json:"now"`
	Context    crmcontracts.AnalyticsContext `json:"context"`
	SelfUserID string                        `json:"self_user_id"`
	Objects    []string                      `json:"objects"`
	want       Plan
}

// Prepare checks that the expected plan passes production admission.
//
//nolint:ireturn // PreparedCase is the certification boundary shared by all decision sites.
func (factory Cases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var c Fixture
	if err := json.Unmarshal(fixture, &c); err != nil {
		return nil, err
	}
	if c.Vocabulary == "" {
		if factory.Vocabulary == nil {
			return nil, errors.New("a search case needs its vocabulary reader")
		}
		vocabulary, err := factory.Vocabulary(c)
		if err != nil {
			return nil, err
		}
		c.Vocabulary = vocabulary
	}
	if c.Question == "" || c.Vocabulary == "" {
		return nil, errors.New("a search case needs a question and vocabulary")
	}
	want, err := Decode(string(expected))
	if err != nil {
		return nil, err
	}
	c.want = want
	return &c, nil
}

// Run sends the fixture through the production planner without loading records.
func (c *Fixture) Run(ctx context.Context, lane aitasks.Completer) (aitasks.Trace, error) {
	request := Request(c.Question, c.Vocabulary)
	trace := aitasks.Trace{Requests: []model.Request{request}}
	response, err := lane.Complete(ctx, request)
	if err != nil {
		return trace, err
	}
	trace.Output = response.Text
	return trace, nil
}

// Evaluate compares the full typed plan, including its scope and filters.
func (c *Fixture) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	got, err := Read(trace.Output, c.Question, c.Vocabulary)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: err.Error()}
	}
	same, err := sameSearchPlanArguments(c.want, got, c.Vocabulary)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}

	if got.Engine != c.want.Engine || got.Count != c.want.Count || !same {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: "the read plan differs from the expected source, period or filters"}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

func sameSearchPlanArguments(want, got Plan, vocabulary string) (bool, error) {
	switch want.Engine {
	case EngineReporting:
		return sameSearchArguments[crmcontracts.ReportingSelection](want.Arguments, got.Arguments)
	case EngineAnalytics:
		return sameSearchArguments[crmcontracts.AnalyticsQuery](want.Arguments, got.Arguments)
	case EngineRecords:
		dates, err := dateFields(vocabulary)
		if err != nil {
			return false, err
		}
		expected, err := orderedSearchPredicates(json.RawMessage(want.Arguments), dates)
		if err != nil {
			return false, err
		}
		actual, err := orderedSearchPredicates(json.RawMessage(got.Arguments), dates)
		return bytes.Equal(expected, actual), err
	default:
		return sameSearchArguments[crmcontracts.WorkspaceQueryRequest](want.Arguments, got.Arguments)
	}
}

func sameSearchArguments[T any](want, got string) (bool, error) {
	var expected, actual T
	if err := DecodeArguments(want, &expected); err != nil {
		return false, err
	}
	if err := DecodeArguments(got, &actual); err != nil {
		return false, err
	}
	return reflect.DeepEqual(expected, actual), nil
}

func orderedSearchPredicates(raw json.RawMessage, dates map[string]bool) ([]byte, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	for _, key := range []string{"plan", "traverse"} {
		if nested, ok := object[key]; ok && string(nested) != jsonNull {
			ordered, err := orderedSearchPredicates(nested, dates)
			if err != nil {
				return nil, err
			}
			object[key] = ordered
		}
	}
	if where, ok := object["where"]; ok {
		var predicates []map[string]json.RawMessage
		if err := json.Unmarshal(where, &predicates); err != nil {
			return nil, err
		}
		var ordered []string
		for _, predicate := range predicates {
			halfOpenDateBound(predicate, dates)
			encoded, err := json.Marshal(predicate)
			if err != nil {
				return nil, err
			}
			ordered = append(ordered, string(encoded))
		}
		slices.Sort(ordered)
		var result []json.RawMessage
		for _, predicate := range ordered {
			result = append(result, json.RawMessage(predicate))
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		object["where"] = encoded
	}
	return json.Marshal(object)
}

// dateFields names the record fields the vocabulary declares as dates in every
// target that has them, the fields whose operand binds as a whole day.
func dateFields(vocabulary string) (map[string]bool, error) {
	var doc struct {
		Records struct {
			Targets []struct {
				Fields []struct{ Name, Kind string } `json:"fields"`
			} `json:"targets"`
		} `json:"record_vocabulary"`
	}
	if err := json.Unmarshal([]byte(vocabulary), &doc); err != nil {
		return nil, err
	}
	dates := map[string]bool{}
	for _, target := range doc.Records.Targets {
		for _, field := range target.Fields {
			if prior, seen := dates[field.Name]; !seen || prior {
				dates[field.Name] = field.Kind == "date"
			}
		}
	}
	return dates, nil
}

// halfOpenDateBound writes a closed bound on a date field as the half-open one
// it equals: "lte 30 Nov" as "lt 1 Dec". Two plans that select one set of days
// then compare equal. An operand that is not a day is left as written.
func halfOpenDateBound(predicate map[string]json.RawMessage, dates map[string]bool) {
	var field, op, day string
	if json.Unmarshal(predicate["field"], &field) != nil || !dates[field] ||
		json.Unmarshal(predicate["op"], &op) != nil || (op != "lte" && op != "gt") ||
		json.Unmarshal(predicate["value"], &day) != nil {
		return
	}
	at, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return
	}
	// Both are ASCII, so a quoted Go string is the JSON string.
	predicate["op"] = json.RawMessage(strconv.Quote(map[string]string{"lte": "lt", "gt": "gte"}[op]))
	predicate["value"] = json.RawMessage(strconv.Quote(at.AddDate(0, 0, 1).Format(time.DateOnly)))
}
