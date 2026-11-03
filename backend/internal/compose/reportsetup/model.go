// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package reportsetup builds the request and reads the reply that turns a
// description in plain words into an initiative's goal or a custom report's
// selection. Every value the reply names is checked against the catalog the
// reader may use. What fails is named back with a reason.
//
// It holds no service and no store. The compose package gathers the catalog
// under the reader's grants, and the certification cases call the same
// Request and Parse production calls.
package reportsetup

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// PromptVersion changes when the prompt or the reply shape changes.
const PromptVersion = "2026-11-03.1"

// The two drawers that ask. An initiative has a goal and a rule; a custom
// report has a selection with metrics and charts.
const (
	SiteInitiative    = "initiative_setup"
	SiteCustomReport  = "custom_report_setup"
	maxNameRunes      = 160
	maxPhraseRunes    = 240
	maxClarifications = 4
	maxUnsupported    = 8
	maxMetrics        = 8
	maxCharts         = 20
)

// The codes a dropped phrase is named back with.
const (
	CodeNotExpressible = "not_expressible"
	CodeUnknownValue   = "unknown_value"
	CodeNotOffered     = "not_offered"
)

var countryShape = regexp.MustCompile(`^[A-Z]{2}$`)

// Option is one value the reader may choose, as the model reads it.
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// MetricOption is a metric and the charts it draws.
type MetricOption struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Charts []string `json:"charts"`
}

// Catalog is everything a proposal may resolve to. Countries are the
// assigned ISO 3166-1 alpha-2 codes; the prompt states the rule and the
// gate checks the list, so the model is not handed 250 codes.
type Catalog struct {
	Products   []Option       `json:"products"`
	Industries []Option       `json:"industries"`
	Countries  []string       `json:"-"`
	Periods    []string       `json:"periods"`
	Metrics    []MetricOption `json:"metrics"`
	Charts     []Option       `json:"charts"`
	Objectives []Option       `json:"objectives,omitempty"`
}

// Input is what the model reads: the words, the catalog and today.
type Input struct {
	Site        string  `json:"site"`
	Description string  `json:"description"`
	Today       string  `json:"today"`
	Catalog     Catalog `json:"catalog"`
	Lang        string  `json:"-"`
}

const system = `You turn a sales manager's description into the setup of a sales initiative or a custom sales report, from a JSON catalog of what they may choose.
Return ONLY a JSON object with these keys:
{"name":"...","filters":[{"field":"product|industry|country|period","value":"...","phrase":"..."}],"metrics":[{"id":"...","phrase":"..."}],"charts":[{"id":"...","phrase":"..."}],"objective":{"metric":"...","target_value":123,"deadline":"YYYY-MM-DD","phrase":"..."} or null,"clarifications":["..."],"unsupported":[{"phrase":"...","reason":"..."}]}
"name" is a short name for what was described. "phrase" is the words of the description a value was read from.
A filter "value" is a catalog id for a product or an industry, an ISO 3166-1 alpha-2 code for a country, or one of the catalog's periods. Name each filter once. Name a filter only when the description asks for it.
"metrics" and "charts" are catalog ids; a chart must be one a chosen metric draws. For site "initiative_setup" name no metrics and no charts.
"objective" is for site "initiative_setup" only: the metric the initiative is measured by, from "objectives", a whole-number target when the description gives one, and a deadline on or after today when it gives one. For "custom_report_setup" it is null.
"clarifications" are the questions the description leaves open, at most four, each one sentence. "unsupported" names each phrase the catalog cannot express and says why in one sentence.
Never invent an id. Never write an id in "name", a phrase, a clarification or a reason.
`

// Request is the one request production sends for both sites.
//
//promptvoice:exempt the reply is a setup; its only prose is a name, the questions left open and a one-sentence reason per unused phrase, which name a field rather than speaking for the product.
func Request(in Input) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         system + "\n" + promptlang.Rule(in.Lang) + "\n" + fence.Rule("description and catalog"),
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

// Proposal is what survives the gate: the values the catalog holds, where
// each came from, and every phrase that did not make it.
type Proposal struct {
	Name           string
	Product        string
	Industry       string
	Country        string
	Period         string
	Metrics        []string
	Blocks         []string
	Objective      *Objective
	Sources        []Source
	Clarifications []string
	Unsupported    []Unsupported
}

// Objective is the metric an initiative is measured by, with the target and
// deadline the words gave, or zero values when they gave none.
type Objective struct {
	Metric      string
	TargetValue int64
	Deadline    string
}

// Source says which words one resolved value was read from.
type Source struct {
	Field  string
	Value  string
	Label  string
	Phrase string
}

// Unsupported is a phrase the proposal cannot use, and why.
type Unsupported struct {
	Phrase string
	Code   string
	Reason string
}

type replyShape struct {
	Name    string `json:"name"`
	Filters []struct {
		Field  string `json:"field"`
		Value  string `json:"value"`
		Phrase string `json:"phrase"`
	} `json:"filters"`
	Metrics []idPhrase `json:"metrics"`
	Charts  []idPhrase `json:"charts"`
	Next    *struct {
		Metric      string  `json:"metric"`
		TargetValue *int64  `json:"target_value"`
		Deadline    *string `json:"deadline"`
		Phrase      string  `json:"phrase"`
	} `json:"objective"`
	Clarifications []string `json:"clarifications"`
	Unsupported    []struct {
		Phrase string `json:"phrase"`
		Reason string `json:"reason"`
	} `json:"unsupported"`
}

type idPhrase struct {
	ID     string `json:"id"`
	Phrase string `json:"phrase"`
}

// Parse reads the reply and keeps what the catalog holds. A value the catalog
// does not carry, or a field this site does not take, is dropped and named
// back. A reply that names nothing at all is refused.
func Parse(reply string, in Input) (Proposal, error) {
	var parsed replyShape
	if err := json.Unmarshal([]byte(ai.Unfence(reply)), &parsed); err != nil {
		return Proposal{}, fmt.Errorf("parse setup proposal: %w", err)
	}
	g := gate{in: in}
	g.name(parsed.Name)
	for _, filter := range parsed.Filters {
		g.filter(filter.Field, filter.Value, filter.Phrase)
	}
	for _, metric := range parsed.Metrics {
		g.metric(metric.ID, metric.Phrase)
	}
	for _, chart := range parsed.Charts {
		g.chart(chart.ID, chart.Phrase)
	}
	if parsed.Next != nil {
		g.objective(parsed.Next.Metric, parsed.Next.TargetValue, parsed.Next.Deadline, parsed.Next.Phrase)
	}
	for _, line := range parsed.Clarifications {
		g.clarification(line)
	}
	for _, phrase := range parsed.Unsupported {
		g.drop(phrase.Phrase, CodeNotExpressible, phrase.Reason)
	}
	if g.out.namesNothing() {
		return Proposal{}, errors.New("setup proposal names nothing")
	}
	return g.out, nil
}

func (p Proposal) namesNothing() bool {
	return p.Name == "" && p.Product == "" && p.Industry == "" && p.Country == "" && p.Period == "" &&
		len(p.Metrics) == 0 && p.Objective == nil && len(p.Clarifications) == 0 && len(p.Unsupported) == 0
}

// gate checks one reply's values against the catalog, in reply order.
type gate struct {
	in  Input
	out Proposal
}

// prose is a phrase cut to its bound, or empty when it spells an id.
func prose(raw string, bound int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || claims.SpellsRecordID(raw) {
		return ""
	}
	if utf8.RuneCountInString(raw) > bound {
		runes := []rune(raw)
		raw = strings.TrimSpace(string(runes[:bound]))
	}
	return raw
}

func (g *gate) name(raw string) {
	if name := prose(raw, maxNameRunes); name != "" {
		g.out.Name = name
		g.keep("name", name, name, name)
	}
}

func (g *gate) keep(field, value, label, phrase string) {
	g.out.Sources = append(g.out.Sources, Source{Field: field, Value: value, Label: label, Phrase: prose(phrase, maxPhraseRunes)})
}

func (g *gate) drop(phrase, code, reason string) {
	if len(g.out.Unsupported) == maxUnsupported {
		return
	}
	phrase, reason = prose(phrase, maxPhraseRunes), prose(reason, maxPhraseRunes)
	if phrase == "" && reason == "" {
		return
	}
	g.out.Unsupported = append(g.out.Unsupported, Unsupported{Phrase: phrase, Code: code, Reason: reason})
}

func (g *gate) clarification(raw string) {
	if line := prose(raw, maxPhraseRunes); line != "" && len(g.out.Clarifications) < maxClarifications {
		g.out.Clarifications = append(g.out.Clarifications, line)
	}
}

// filter resolves one dimension. A product or an industry may be named by
// its id or its label. A country is its code, and a period is one of the
// catalog's. A dimension named twice keeps its first value.
func (g *gate) filter(field, value, phrase string) {
	value = strings.TrimSpace(value)
	switch field {
	case "product":
		g.option(&g.out.Product, field, value, phrase, g.in.Catalog.Products)
	case "industry":
		g.option(&g.out.Industry, field, value, phrase, g.in.Catalog.Industries)
	case "country":
		code := strings.ToUpper(value)
		if !countryShape.MatchString(code) || !slices.Contains(g.in.Catalog.Countries, code) {
			g.drop(phrase, CodeUnknownValue, fmt.Sprintf("%q is not a country code", value))
			return
		}
		g.set(&g.out.Country, field, code, code, phrase)
	case "period":
		g.option(&g.out.Period, field, value, phrase, periodOptions(g.in.Catalog.Periods))
	default:
		g.drop(phrase, CodeNotOffered, fmt.Sprintf("there is no filter called %q", field))
	}
}

func periodOptions(periods []string) []Option {
	out := make([]Option, 0, len(periods))
	for _, period := range periods {
		out = append(out, Option{ID: period, Label: period})
	}
	return out
}

func (g *gate) option(into *string, field, value, phrase string, offered []Option) {
	found, err := resolve(field, value, offered)
	if err != nil {
		g.drop(phrase, CodeUnknownValue, err.Error())
		return
	}
	g.set(into, field, found.ID, found.Label, phrase)
}

// set keeps the first value of a dimension. A second, different value is
// named back rather than silently lost, so a contradictory reply shows.
func (g *gate) set(into *string, field, value, label, phrase string) {
	switch {
	case *into == "":
		*into = value
		g.keep(field, value, label, phrase)
	case *into != value:
		g.drop(phrase, CodeNotOffered, fmt.Sprintf("the %s is already set; a report has one", field))
	}
}

// resolve finds an option by id, or by its label when the model answered
// with the name it read. A label two options share resolves to neither.
func resolve(field, value string, offered []Option) (Option, error) {
	for _, option := range offered {
		if option.ID == value {
			return option, nil
		}
	}
	var found []Option
	for _, option := range offered {
		if strings.EqualFold(option.Label, value) {
			found = append(found, option)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return Option{}, fmt.Errorf("there is no %s called %q", field, value)
	default:
		return Option{}, fmt.Errorf("more than one %s is called %q", field, value)
	}
}

func (g *gate) metric(id, phrase string) {
	if g.in.Site == SiteInitiative {
		g.drop(phrase, CodeNotOffered, "an initiative has one objective, not a list of metrics")
		return
	}
	metric, ok := g.metricOption(id)
	if !ok {
		g.drop(phrase, CodeUnknownValue, fmt.Sprintf("there is no metric called %q", id))
		return
	}
	if len(g.out.Metrics) < maxMetrics && !slices.Contains(g.out.Metrics, metric.ID) {
		g.out.Metrics = append(g.out.Metrics, metric.ID)
		g.keep("metric", metric.ID, metric.Label, phrase)
	}
}

func (g *gate) metricOption(id string) (MetricOption, bool) {
	id = strings.TrimSpace(id)
	for _, metric := range g.in.Catalog.Metrics {
		if metric.ID == id || strings.EqualFold(metric.Label, id) {
			return metric, true
		}
	}
	return MetricOption{}, false
}

// chart keeps a chart one kept metric draws. Charts are read after every
// metric of the reply, so their order in the reply does not matter.
func (g *gate) chart(id, phrase string) {
	if g.in.Site == SiteInitiative {
		g.drop(phrase, CodeNotOffered, "an initiative draws its own charts")
		return
	}
	chart, err := resolve("chart", strings.TrimSpace(id), g.in.Catalog.Charts)
	if err != nil {
		g.drop(phrase, CodeUnknownValue, err.Error())
		return
	}
	if !g.drawn(chart.ID) {
		g.drop(phrase, CodeNotOffered, fmt.Sprintf("no chosen metric draws %s", chart.Label))
		return
	}
	if len(g.out.Blocks) < maxCharts && !slices.Contains(g.out.Blocks, chart.ID) {
		g.out.Blocks = append(g.out.Blocks, chart.ID)
		g.keep("chart", chart.ID, chart.Label, phrase)
	}
}

func (g *gate) drawn(chart string) bool {
	for _, id := range g.out.Metrics {
		if metric, ok := g.metricOption(id); ok && slices.Contains(metric.Charts, chart) {
			return true
		}
	}
	return false
}

// objective keeps the initiative's measure. A target below one or a deadline
// before today is dropped on its own; the metric stays.
func (g *gate) objective(metric string, target *int64, deadline *string, phrase string) {
	if g.in.Site != SiteInitiative {
		g.drop(phrase, CodeNotOffered, "a custom report has no objective")
		return
	}
	found, err := resolve("objective", strings.TrimSpace(metric), g.in.Catalog.Objectives)
	if err != nil {
		g.drop(phrase, CodeUnknownValue, err.Error())
		return
	}
	g.out.Objective = &Objective{Metric: found.ID}
	g.keep("objective", found.ID, found.Label, phrase)
	if target != nil {
		if *target < 1 {
			g.drop(phrase, CodeUnknownValue, "a target is a whole number above zero")
		} else {
			g.out.Objective.TargetValue = *target
			g.keep("target_value", fmt.Sprint(*target), fmt.Sprint(*target), phrase)
		}
	}
	if deadline != nil {
		g.deadline(*deadline, phrase)
	}
}

func (g *gate) deadline(raw, phrase string) {
	day, err := time.Parse(time.DateOnly, strings.TrimSpace(raw))
	if err != nil {
		g.drop(phrase, CodeUnknownValue, fmt.Sprintf("%q is not a date", raw))
		return
	}
	if today, err := time.Parse(time.DateOnly, g.in.Today); err == nil && day.Before(today) {
		g.drop(phrase, CodeUnknownValue, "the deadline is before today")
		return
	}
	g.out.Objective.Deadline = day.Format(time.DateOnly)
	g.keep("deadline", g.out.Objective.Deadline, g.out.Objective.Deadline, phrase)
}
