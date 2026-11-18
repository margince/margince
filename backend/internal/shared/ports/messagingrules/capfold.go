// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingrules

import "math/big"

// tighterCap picks the cap that allows no more messages than the other in any
// stretch of time. Nil means uncapped, so a nil on either side yields the
// other's cap. When neither holds the other everywhere, it answers one cap
// that holds both.
func tighterCap(a, b *FrequencyCap) *FrequencyCap {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	// A degenerate cap is the strictest thing there is. It is handled first,
	// because Register is exported and a core-built value never sees Validate.
	if degenerate(a) {
		return a
	}
	if degenerate(b) {
		return b
	}
	switch {
	case !allowsMore(a, b):
		return a
	case !allowsMore(b, a):
		return b
	}
	// Two per 3h lets two out at once and one per hour does not, while one per
	// hour lets more out over a day. The smaller count over the longer window
	// holds both, at the price of being stricter than either.
	return &FrequencyCap{Messages: min(a.Messages, b.Messages), Window: max(a.Window, b.Window)}
}

// maxCapSteps bounds the window steps allowsMore compares. Past it the answer
// is "allows more", the direction that keeps the other cap.
const maxCapSteps = 1 << 16

// allowsMore reports whether some stretch of time lets more messages through
// under c than under other.
//
// A cap of n per w lets n·⌈L/w⌉ through in a stretch of length L. c gains most
// just after each multiple of its own window, so only those stretches are
// compared: k·w + ε lets n_c·(k+1) through under c, and n_o·(⌊k·w_c/w_o⌋+1)
// under other.
func allowsMore(c, other *FrequencyCap) bool {
	nc, wc := big.NewInt(int64(c.Messages)), big.NewInt(int64(c.Window))
	no, wo := big.NewInt(int64(other.Messages)), big.NewInt(int64(other.Window))
	// Over long stretches only the rates count: n_c/w_c against n_o/w_o.
	rateGap := new(big.Int).Sub(new(big.Int).Mul(no, wc), new(big.Int).Mul(nc, wo))
	if rateGap.Sign() < 0 {
		return true
	}
	steps := new(big.Int)
	if rateGap.Sign() > 0 {
		// Past n_c·w_o / gap steps the slower rate has outrun c's bursts.
		steps.Quo(new(big.Int).Mul(nc, wo), rateGap)
		steps.Add(steps, big.NewInt(1))
	} else {
		// Equal rates repeat with a period of w_o / gcd(w_c, w_o) steps.
		steps.Quo(wo, new(big.Int).GCD(nil, nil, wc, wo))
	}
	if !steps.IsInt64() || steps.Int64() > maxCapSteps {
		return true
	}
	under, over := new(big.Int), new(big.Int)
	for k := range steps.Int64() {
		step := big.NewInt(k)
		under.Mul(nc, new(big.Int).Add(step, big.NewInt(1)))
		over.Quo(new(big.Int).Mul(step, wc), wo)
		over.Mul(no, over.Add(over, big.NewInt(1)))
		if under.Cmp(over) > 0 {
			return true
		}
	}
	return false
}
