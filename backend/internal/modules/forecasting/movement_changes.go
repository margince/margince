// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// amountChanged answers whether the deal's own money moved, including a deal
// that BECAME unpriced: its full prior value leaves through this bucket rather
// than through an exclusion nobody drew.
func amountChanged(was, now Contribution) bool {
	switch {
	case was.AmountMinor == nil && now.AmountMinor == nil:
		return false
	case was.AmountMinor == nil || now.AmountMinor == nil:
		return true
	default:
		return *was.AmountMinor != *now.AmountMinor || was.Currency != now.Currency
	}
}

// stageMoved answers whether the deal changed stage. A side frozen without its
// stage counts as moved, because nothing says it stayed.
func stageMoved(was, now Contribution) bool {
	return was.StageID == "" || now.StageID == "" || was.StageID != now.StageID
}

// weightRecordChanged answers whether the chance or the risk effect moved on a
// deal that stayed in its stage.
func weightRecordChanged(was, now Contribution) bool {
	if stageMoved(was, now) {
		return false
	}
	return was.StageProbability != now.StageProbability || !sameEffect(was.Effect, now.Effect)
}

func sameEffect(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// fxChanged answers whether only the conversion moved. Checked after the
// amount, so a deal whose price AND rate both moved reads as a repricing —
// which is what happened to it commercially.
func fxChanged(was, now Contribution) bool {
	if was.BaseMinor == nil || now.BaseMinor == nil {
		return was.BaseMinor != now.BaseMinor
	}
	return *was.BaseMinor != *now.BaseMinor
}
