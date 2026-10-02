// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// The vocabularies OpenRouter accepts for the enumerated fields. A value
// outside them is dropped in silence by the broker, so it is refused here.
var (
	dataCollections = []string{"allow", "deny"}
	sortPartitions  = []string{"model", "none"}
)

// Validate refuses a preference the broker would silently ignore, naming each
// bad key by its path under path.
//
// Exported because the config file is not the only door: the certification
// lane takes these preferences from an environment variable, and a run that
// accepted a misspelt value would report the untuned baseline under a tuned
// run's name. One check, every door.
func (r *OpenRouterRouting) Validate(path string) error {
	if r == nil {
		return nil
	}
	return joinFaults(r.Provider.validate(joinPath(path, "provider")), r.Reasoning.validate(joinPath(path, "reasoning")))
}

func (p OpenRouterProvider) validate(path string) error {
	errs := []error{
		oneOf(joinPath(path, "data_collection"), p.DataCollection, dataCollections),
		p.Sort.validate(joinPath(path, "sort")),
		p.MaxPrice.validate(joinPath(path, "max_price")),
		p.PreferredMinThroughput.validate(joinPath(path, "preferred_min_throughput")),
		p.PreferredMaxLatency.validate(joinPath(path, "preferred_max_latency")),
	}
	// The generated schema declares these arrays minItems:1 and uniqueItems, so
	// the parser refuses the same shapes: an editor and a runtime that authorize
	// different configs is the drift the parity gate exists to catch.
	for _, list := range []struct {
		key     string
		entries []string
	}{{"order", p.Order}, {"only", p.Only}, {"ignore", p.Ignore}, {"quantizations", p.Quantizations}} {
		errs = append(errs, refuseEmptyOrRepeated(joinPath(path, list.key), list.entries))
	}
	for i, q := range p.Quantizations {
		errs = append(errs, oneOf(fmt.Sprintf("%s.quantizations[%d]", path, i), q, quantizationLevels))
	}
	return joinFaults(errs...)
}

func (s *OpenRouterSort) validate(path string) error {
	if s == nil {
		return nil
	}
	by := path
	if s.asObject || s.Partition != "" {
		by = joinPath(path, "by")
	}
	if s.By == "" {
		return invalidAt(by, "names no order; write one of "+strings.Join(sortOrders, ", "))
	}
	return joinFaults(oneOf(by, s.By, sortOrders), oneOf(joinPath(path, "partition"), s.Partition, sortPartitions))
}

func (p *OpenRouterPrice) validate(path string) error {
	if p == nil {
		return nil
	}
	return joinFaults(
		nonNegative(joinPath(path, "prompt"), p.Prompt), nonNegative(joinPath(path, "completion"), p.Completion),
		nonNegative(joinPath(path, "request"), p.Request), nonNegative(joinPath(path, "image"), p.Image),
	)
}

func (p *OpenRouterPctile) validate(path string) error {
	if p == nil {
		return nil
	}
	if p.All != nil {
		return nonNegative(path, p.All)
	}
	if p.P50 == nil && p.P75 == nil && p.P90 == nil && p.P99 == nil {
		return invalidAt(path, "is written with no percentile; give a number, or one of p50, p75, p90, p99")
	}
	return joinFaults(
		nonNegative(joinPath(path, "p50"), p.P50), nonNegative(joinPath(path, "p75"), p.P75),
		nonNegative(joinPath(path, "p90"), p.P90), nonNegative(joinPath(path, "p99"), p.P99),
	)
}

func (r *OpenRouterReasoning) validate(path string) error {
	if r == nil {
		return nil
	}
	// OpenRouter takes one budget or the other; sent both, it picks one without
	// saying which, so the config that ran is not the config that was written.
	if r.Effort != "" && r.MaxTokens != nil {
		return invalidAt(path, "takes effort or max_tokens, not both; keep the one you mean")
	}
	errs := []error{oneOf(joinPath(path, "effort"), r.Effort, reasoningEfforts)}
	if r.MaxTokens != nil && *r.MaxTokens <= 0 {
		errs = append(errs, invalidAt(joinPath(path, "max_tokens"), fmt.Sprintf("must be a positive number of tokens. You wrote %d.", *r.MaxTokens)))
	}
	return joinFaults(errs...)
}

// oneOf refuses a written value outside its vocabulary; "" is unset and passes.
func oneOf(path, value string, vocabulary []string) error {
	if value == "" || slices.Contains(vocabulary, value) {
		return nil
	}
	return invalidAt(path, fmt.Sprintf("must be one of %s. You wrote %q.", strings.Join(vocabulary, ", "), value))
}

// nonNegative refuses a negative or non-finite number. NaN and the infinities
// fail later and worse: JSON cannot carry them, so every request would fail at
// encode time on a config that booted.
func nonNegative(path string, v *float64) error {
	switch {
	case v == nil:
		return nil
	case math.IsNaN(*v) || math.IsInf(*v, 0):
		return invalidAt(path, "must be a finite number")
	case *v < 0:
		return invalidAt(path, fmt.Sprintf("must not be negative. You wrote %g.", *v))
	}
	return nil
}
