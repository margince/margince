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
	var medians []float64
	for _, e := range measured {
		if e != nil && e.Calls != nil {
			medians = append(medians, e.Calls.Median)
		}
	}
	if len(medians) == 0 {
		return Dash
	}
	slices.Sort(medians)
	middle := len(medians) / 2
	median := medians[middle]
	if len(medians)%2 == 0 {
		median = (medians[middle-1] + medians[middle]) / 2
	}
	return fmt.Sprintf("%s over %d", strconv.FormatFloat(median, 'f', -1, 64), len(medians))
}
