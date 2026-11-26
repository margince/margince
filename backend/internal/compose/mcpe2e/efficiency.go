// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"fmt"
	"slices"
	"strconv"
)

// Efficiency is how directly a verdict's runs finished, as e2e/llm/check.py
// records it. It is a score beside the verdict and never moves PASS or FAIL.
type Efficiency struct {
	Runs  int     `json:"runs"`
	Calls *Spread `json:"calls"`
	Turns *Spread `json:"turns"`
}

// Spread is one metric's median and range across a verdict's finished runs.
type Spread struct {
	Median float64 `json:"median"`
	Min    int     `json:"min"`
	Max    int     `json:"max"`
}

// Dash is the cell of a verdict recorded before the lane measured efficiency.
const Dash = "-"

// CallsCell renders the median calls with their range.
func (e *Efficiency) CallsCell() string {
	if e == nil {
		return Dash
	}
	return e.Calls.cell()
}

// TurnsCell renders the median model turns with their range.
func (e *Efficiency) TurnsCell() string {
	if e == nil {
		return Dash
	}
	return e.Turns.cell()
}

func (s *Spread) cell() string {
	if s == nil {
		return Dash
	}
	median := strconv.FormatFloat(s.Median, 'f', -1, 64)
	if s.Min == s.Max {
		return median
	}
	return fmt.Sprintf("%s (%d–%d)", median, s.Min, s.Max)
}

// CallsPerCase is one model's median of its cases' median calls, over the
// cases that measured them, so two models compare on how directly they finish.
func CallsPerCase(measured []*Efficiency) string {
	median, cases := medianOver(measured, func(e *Efficiency) *Spread { return e.Calls })
	if cases == 0 {
		return Dash
	}
	return fmt.Sprintf("%s over %d", median, cases)
}

// MedianCalls and MedianTurns are the same median, bare, for the page's summary
// table. A route that reports no turn count, as codex does, has Dash and never 0.
func MedianCalls(measured []*Efficiency) string {
	median, _ := medianOver(measured, func(e *Efficiency) *Spread { return e.Calls })
	return median
}

// MedianTurns is MedianCalls for the model's turns.
func MedianTurns(measured []*Efficiency) string {
	median, _ := medianOver(measured, func(e *Efficiency) *Spread { return e.Turns })
	return median
}

// medianOver is the median of one metric's per-case medians and how many cases
// carried it; Dash and 0 when none did.
func medianOver(measured []*Efficiency, metric func(*Efficiency) *Spread) (string, int) {
	var medians []float64
	for _, e := range measured {
		if e != nil && metric(e) != nil {
			medians = append(medians, metric(e).Median)
		}
	}
	if len(medians) == 0 {
		return Dash, 0
	}
	slices.Sort(medians)
	middle := len(medians) / 2
	median := medians[middle]
	if len(medians)%2 == 0 {
		median = (medians[middle-1] + medians[middle]) / 2
	}
	return strconv.FormatFloat(median, 'f', -1, 64), len(medians)
}
