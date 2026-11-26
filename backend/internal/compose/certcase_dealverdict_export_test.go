// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/dealverdict"
)

// DealVerdictCaseInput is the input and expected amounts of a prepared
// deal_verdict case. The guard gate reads the corpus through aicert, so it
// lives outside this package.
func DealVerdictCaseInput(pc aitasks.PreparedCase) (dealverdict.Input, []int64, bool) {
	c, ok := pc.(*dealVerdictCase)
	if !ok {
		return dealverdict.Input{}, nil, false
	}
	var amounts []int64
	for _, w := range c.expected {
		if w.Kind == dealverdict.KindSetAmount {
			amounts = append(amounts, w.Amount)
		}
	}
	return c.in, amounts, true
}
